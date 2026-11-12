// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
import { HttpErrorResponse } from '@angular/common/http';
import { BehaviorSubject, of, Subject, throwError } from 'rxjs';
import { ProjectService } from '../../../../ng-swagger-gen/services/project.service';
import { RepositoryService } from '../../../../ng-swagger-gen/services/repository.service';
import { ArtifactService } from '../../../../ng-swagger-gen/services/artifact.service';
import { LabelService } from '../../../../ng-swagger-gen/services/label.service';
import { SkipSessionRenewalService } from '../../services/skip-session-renewal.service';
import {
    BookmarkHealthService,
    BookmarkStatus,
    statusFromError,
} from './bookmark-health.service';
import { Bookmark, BookmarkKind, BookmarkService } from './bookmark.service';

describe('BookmarkHealthService', () => {
    let projectService: jasmine.SpyObj<ProjectService>;
    let repositoryService: jasmine.SpyObj<RepositoryService>;
    let artifactService: jasmine.SpyObj<ArtifactService>;
    let labelService: jasmine.SpyObj<LabelService>;
    let skip: SkipSessionRenewalService;
    let service: BookmarkHealthService;
    let statuses: ReadonlyMap<string, BookmarkStatus>;
    let owner: BehaviorSubject<string>;
    let bookmarks: BehaviorSubject<Bookmark[]>;

    const base = {
        projectId: 7,
        projectName: 'library',
        label: 'x',
        defaultLabel: 'x',
        createdAt: 0,
    };
    const project: Bookmark = { ...base, id: 'p', kind: BookmarkKind.PROJECT };
    const repo: Bookmark = {
        ...base,
        id: 'r',
        kind: BookmarkKind.REPOSITORY,
        repoName: 'team/app',
    };
    const tag: Bookmark = {
        ...base,
        id: 't',
        kind: BookmarkKind.TAG,
        repoName: 'team/app',
        tag: 'v1',
    };
    const labelFilter: Bookmark = {
        ...base,
        id: 'l',
        kind: BookmarkKind.ARTIFACT_FILTER,
        repoName: 'team/app',
        query: 'labels=(3)',
    };
    const notFound = () => throwError(new HttpErrorResponse({ status: 404 }));

    beforeEach(() => {
        projectService = jasmine.createSpyObj('ProjectService', ['getProject']);
        projectService.getProject.and.returnValue(
            of({ name: 'library' } as any)
        );
        repositoryService = jasmine.createSpyObj('RepositoryService', [
            'listRepositories',
        ]);
        repositoryService.listRepositories.and.returnValue(of([{} as any]));
        artifactService = jasmine.createSpyObj('ArtifactService', [
            'getArtifact',
        ]);
        artifactService.getArtifact.and.returnValue(of({} as any));
        labelService = jasmine.createSpyObj('LabelService', [
            'GetLabelByID',
            'ListLabels',
        ]);
        labelService.ListLabels.and.returnValue(of([]));
        labelService.GetLabelByID.and.returnValue(of({} as any));
        skip = new SkipSessionRenewalService();
        owner = new BehaviorSubject('harbor-bookmarks:1');
        bookmarks = new BehaviorSubject<Bookmark[]>([]);
        service = new BookmarkHealthService(
            projectService,
            repositoryService,
            artifactService,
            labelService,
            skip,
            {
                owner$: owner.asObservable(),
                bookmarks$: bookmarks.asObservable(),
            } as BookmarkService
        );
        service.statuses$.subscribe(s => (statuses = s));
    });

    it('maps HTTP errors to statuses', () => {
        expect(statusFromError({ status: 404 })).toBe(BookmarkStatus.MISSING);
        expect(statusFromError({ status: 403 })).toBe(BookmarkStatus.FORBIDDEN);
        expect(statusFromError({ status: 500 })).toBe(BookmarkStatus.UNKNOWN);
        expect(statusFromError(undefined)).toBe(BookmarkStatus.UNKNOWN);
    });

    it('looks the project up once by id and probes each kind', () => {
        repositoryService.listRepositories.and.returnValue(of([]));
        artifactService.getArtifact.and.returnValue(
            throwError(new HttpErrorResponse({ status: 403 }))
        );

        service.checkAll([project, repo, tag], true);

        expect(projectService.getProject).toHaveBeenCalledOnceWith({
            projectNameOrId: '7',
        });
        expect(repositoryService.listRepositories).toHaveBeenCalledWith({
            projectName: 'library',
            q: encodeURIComponent('name=library/team/app'),
            page: 1,
            pageSize: 1,
        });
        expect(artifactService.getArtifact.calls.mostRecent().args[0]).toEqual(
            jasmine.objectContaining({
                projectName: 'library',
                repositoryName: 'team%252Fapp',
                reference: 'v1',
            })
        );
        expect(service.statusOf(statuses, project)).toBe(BookmarkStatus.OK);
        expect(service.statusOf(statuses, repo)).toBe(BookmarkStatus.MISSING);
        expect(service.statusOf(statuses, tag)).toBe(BookmarkStatus.FORBIDDEN);
    });

    it('applies a project error to all of its bookmarks', () => {
        projectService.getProject.and.returnValue(
            throwError(new HttpErrorResponse({ status: 403 }))
        );
        service.checkAll([project, tag], true);
        expect(service.statusOf(statuses, project)).toBe(
            BookmarkStatus.FORBIDDEN
        );
        expect(service.statusOf(statuses, tag)).toBe(BookmarkStatus.FORBIDDEN);
        expect(artifactService.getArtifact).not.toHaveBeenCalled();
    });

    it('treats a project re-created under the same id as missing', () => {
        projectService.getProject.and.returnValue(of({ name: 'other' } as any));
        service.checkAll([project, repo], true);
        expect(service.statusOf(statuses, project)).toBe(
            BookmarkStatus.MISSING
        );
        expect(service.statusOf(statuses, repo)).toBe(BookmarkStatus.MISSING);
        expect(repositoryService.listRepositories).not.toHaveBeenCalled();
    });

    it('flags a label filter whose label was deleted', () => {
        labelService.GetLabelByID.and.returnValue(notFound());
        service.checkAll([labelFilter], true);
        expect(labelService.GetLabelByID).toHaveBeenCalledWith({ labelId: 3 });
        expect(service.statusOf(statuses, labelFilter)).toBe(
            BookmarkStatus.MISSING
        );
    });

    it('accepts a global label a non-admin may only list', () => {
        labelService.GetLabelByID.and.returnValue(
            throwError(new HttpErrorResponse({ status: 403 }))
        );
        labelService.ListLabels.and.returnValue(of([{ id: 3 } as any]));
        service.checkAll([labelFilter], true);
        expect(labelService.ListLabels).toHaveBeenCalledWith({
            scope: 'g',
            q: encodeURIComponent('id=3'),
        });
        expect(service.statusOf(statuses, labelFilter)).toBe(BookmarkStatus.OK);
    });

    it('keeps 403 when the label is not a visible global label', () => {
        labelService.GetLabelByID.and.returnValue(
            throwError(new HttpErrorResponse({ status: 403 }))
        );
        service.checkAll([labelFilter], true);
        expect(service.statusOf(statuses, labelFilter)).toBe(
            BookmarkStatus.FORBIDDEN
        );
    });

    it('drops a probe result that a newer probe of the bookmark replaced', () => {
        const slow = new Subject<any>();
        repositoryService.listRepositories.and.returnValues(
            slow.asObservable(),
            of([{} as any])
        );
        service.checkAll([repo], true);
        service.checkAll([repo], true);
        expect(service.statusOf(statuses, repo)).toBe(BookmarkStatus.OK);
        slow.next([]);
        slow.complete();
        expect(service.statusOf(statuses, repo)).toBe(BookmarkStatus.OK);
    });

    it('marks probes as background requests', () => {
        let skipped: boolean;
        projectService.getProject.and.callFake(() => {
            skipped = skip.shouldSkip;
            return of({ name: 'library' } as any);
        });
        service.checkAll([project], true);
        expect(projectService.getProject).toHaveBeenCalled();
        // the flag is raised when the request is subscribed, not when it is built
        expect(skipped).toBeFalse();
        expect(skip.shouldSkip).toBeTrue();
    });

    it('throttles unforced full checks', () => {
        service.checkAll([project], true);
        service.checkAll([project]);
        expect(projectService.getProject).toHaveBeenCalledTimes(1);
        service.checkAll([project], true);
        expect(projectService.getProject).toHaveBeenCalledTimes(2);
    });

    it('probes added bookmarks and forgets removed ones', () => {
        // the signed-in user's list arrives right after the owner
        owner.next('harbor-bookmarks:1');
        bookmarks.next([project]);
        expect(projectService.getProject).not.toHaveBeenCalled();

        repositoryService.listRepositories.and.returnValue(of([]));
        bookmarks.next([project, repo]);
        expect(service.statusOf(statuses, repo)).toBe(BookmarkStatus.MISSING);

        bookmarks.next([project]);
        expect(statuses.size).toBe(0);

        repositoryService.listRepositories.and.returnValue(of([{} as any]));
        bookmarks.next([project, repo]);
        expect(service.statusOf(statuses, repo)).toBe(BookmarkStatus.OK);
    });

    it('forgets statuses and the throttle when the user changes', () => {
        service.checkAll([project], true);
        expect(service.statusOf(statuses, project)).toBe(BookmarkStatus.OK);

        owner.next('harbor-bookmarks:2');
        expect(service.statusOf(statuses, project)).toBe(
            BookmarkStatus.UNKNOWN
        );
        // the new user's list is covered by checkAll, not probed as additions
        bookmarks.next([project]);
        expect(projectService.getProject).toHaveBeenCalledTimes(1);
        service.checkAll([project]);
        expect(projectService.getProject).toHaveBeenCalledTimes(2);
    });

    it('ignores probe results that arrive after the user changed', () => {
        const pending = new Subject<any>();
        projectService.getProject.and.returnValue(pending);
        service.checkAll([project], true);
        owner.next('harbor-bookmarks:2');
        pending.error(new HttpErrorResponse({ status: 403 }));
        expect(service.statusOf(statuses, project)).toBe(
            BookmarkStatus.UNKNOWN
        );
    });

    it('accepts reported statuses', () => {
        service.report(tag, BookmarkStatus.MISSING);
        expect(service.statusOf(statuses, tag)).toBe(BookmarkStatus.MISSING);
    });
});
