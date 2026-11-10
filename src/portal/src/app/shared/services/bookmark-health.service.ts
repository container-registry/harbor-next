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
import { Injectable } from '@angular/core';
import { BehaviorSubject, from, Observable, of } from 'rxjs';
import {
    catchError,
    map,
    mergeMap,
    shareReplay,
    switchMap,
} from 'rxjs/operators';
import { ProjectService } from '../../../../ng-swagger-gen/services/project.service';
import { RepositoryService } from '../../../../ng-swagger-gen/services/repository.service';
import { ArtifactService } from '../../../../ng-swagger-gen/services/artifact.service';
import { LabelService } from '../../../../ng-swagger-gen/services/label.service';
import { Project } from '../../../../ng-swagger-gen/models/project';
import {
    SkipSessionRenewalService,
    skipSessionRenewal,
} from '../../services/skip-session-renewal.service';
import { dbEncodeURIComponent } from '../units/utils';
import {
    Bookmark,
    BookmarkKind,
    BookmarkService,
    BookmarkTarget,
    bookmarkKey,
} from './bookmark.service';

export enum BookmarkStatus {
    OK = 'ok',
    MISSING = 'missing',
    // Harbor answers non-admins with 403 for deleted projects too.
    FORBIDDEN = 'forbidden',
    UNKNOWN = 'unknown',
}

export const BOOKMARK_RECHECK_INTERVAL_MS = 2 * 60 * 1000;
const PROBE_CONCURRENCY = 4;
const LABEL_QUERY = /^labels=\((\d+)\)$/;

export function statusFromError(error: any): BookmarkStatus {
    switch (error?.status) {
        case 404:
            return BookmarkStatus.MISSING;
        case 403:
            return BookmarkStatus.FORBIDDEN;
        default:
            return BookmarkStatus.UNKNOWN;
    }
}

@Injectable({
    providedIn: 'root',
})
export class BookmarkHealthService {
    // A Map, not an object literal: keys derive from stored and routed values.
    private readonly statuses = new BehaviorSubject<
        ReadonlyMap<string, BookmarkStatus>
    >(new Map());
    readonly statuses$ = this.statuses.asObservable();
    private lastFullCheck = 0;
    // Probes started for a previous user must not write into the new user's statuses.
    private generation = 0;
    // null until the signed-in user's list arrives; that list is covered by checkAll.
    private known: Set<string> | null = null;

    constructor(
        private projectService: ProjectService,
        private repositoryService: RepositoryService,
        private artifactService: ArtifactService,
        private labelService: LabelService,
        private skipSessionRenewalService: SkipSessionRenewalService,
        bookmarkService: BookmarkService
    ) {
        bookmarkService.owner$.subscribe(() => {
            this.generation++;
            this.lastFullCheck = 0;
            this.known = null;
            this.statuses.next(new Map());
        });
        bookmarkService.bookmarks$.subscribe(bookmarks =>
            this.track(bookmarks)
        );
    }

    // The probes are background traffic: they must not keep an idle session alive.
    checkAll(bookmarks: Bookmark[], force: boolean = false): void {
        if (
            !force &&
            Date.now() - this.lastFullCheck < BOOKMARK_RECHECK_INTERVAL_MS
        ) {
            return;
        }
        this.lastFullCheck = Date.now();
        this.probeAll(bookmarks);
    }

    check(bookmarks: Bookmark[]): void {
        this.probeAll(bookmarks);
    }

    report(target: BookmarkTarget, status: BookmarkStatus): void {
        if (target) {
            this.set(bookmarkKey(target), status);
        }
    }

    statusOf(
        statuses: ReadonlyMap<string, BookmarkStatus>,
        target: BookmarkTarget
    ): BookmarkStatus {
        return statuses.get(bookmarkKey(target)) || BookmarkStatus.UNKNOWN;
    }

    // New bookmarks are probed at once and removed ones forget their status, so a
    // re-added bookmark never shows a stale marker.
    private track(bookmarks: Bookmark[]): void {
        const keys = new Set(bookmarks.map(bookmarkKey));
        const previous = this.known;
        this.known = keys;
        if (!previous) {
            return;
        }
        const added = bookmarks.filter(b => !previous.has(bookmarkKey(b)));
        const removed = [...previous].filter(k => !keys.has(k));
        if (removed.some(k => this.statuses.value.has(k))) {
            const next = new Map(this.statuses.value);
            removed.forEach(k => next.delete(k));
            this.statuses.next(next);
        }
        if (added.length) {
            this.probeAll(added);
        }
    }

    private probeAll(bookmarks: Bookmark[]): void {
        const generation = this.generation;
        const projects = new Map<number, Observable<Project>>();
        const projectOf = (id: number) => {
            if (!projects.has(id)) {
                projects.set(
                    id,
                    this.background(
                        this.projectService.getProject({
                            projectNameOrId: String(id),
                        })
                    ).pipe(shareReplay(1))
                );
            }
            return projects.get(id);
        };
        from(bookmarks)
            .pipe(
                mergeMap(
                    b =>
                        projectOf(b.projectId).pipe(
                            // Links use the id; a project re-created under the old
                            // name gets a new id, so the name must match too.
                            switchMap(project =>
                                project?.name === b.projectName
                                    ? this.probe(b)
                                    : of(BookmarkStatus.MISSING)
                            ),
                            catchError(error => of(statusFromError(error))),
                            map(status => ({ key: bookmarkKey(b), status }))
                        ),
                    PROBE_CONCURRENCY
                )
            )
            .subscribe(({ key, status }) => {
                if (generation === this.generation) {
                    this.set(key, status);
                }
            });
    }

    private probe(b: Bookmark): Observable<BookmarkStatus> {
        const repositoryName = dbEncodeURIComponent(b.repoName);
        switch (b.kind) {
            case BookmarkKind.PROJECT:
            case BookmarkKind.REPOSITORY_FILTER:
                return of(BookmarkStatus.OK);
            case BookmarkKind.REPOSITORY:
                return this.repositoryExists(b);
            case BookmarkKind.ARTIFACT_FILTER: {
                const label = LABEL_QUERY.exec(b.query || '');
                if (!label) {
                    return this.repositoryExists(b);
                }
                return this.background(
                    this.labelService.GetLabelByID({ labelId: +label[1] })
                ).pipe(switchMap(() => this.repositoryExists(b)));
            }
            case BookmarkKind.TAG:
                return this.background(
                    this.artifactService.getArtifact({
                        projectName: b.projectName,
                        repositoryName,
                        reference: b.tag,
                        withTag: false,
                        withScanOverview: false,
                        withSbomOverview: false,
                        withLabel: false,
                        withAccessory: false,
                    })
                ).pipe(map(() => BookmarkStatus.OK));
            default:
                return of(BookmarkStatus.UNKNOWN);
        }
    }

    // Listing, unlike GET repository, is allowed for non-members of public projects.
    private repositoryExists(b: Bookmark): Observable<BookmarkStatus> {
        return this.background(
            this.repositoryService.listRepositories({
                projectName: b.projectName,
                q: encodeURIComponent(`name=${b.projectName}/${b.repoName}`),
                page: 1,
                pageSize: 1,
            })
        ).pipe(
            map(repos =>
                repos?.length ? BookmarkStatus.OK : BookmarkStatus.MISSING
            )
        );
    }

    private background<T>(request: Observable<T>): Observable<T> {
        return request.pipe(skipSessionRenewal(this.skipSessionRenewalService));
    }

    private set(key: string, status: BookmarkStatus): void {
        if (this.statuses.value.get(key) !== status) {
            this.statuses.next(new Map(this.statuses.value).set(key, status));
        }
    }
}
