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
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, Router } from '@angular/router';
import { HttpErrorResponse } from '@angular/common/http';
import { BehaviorSubject, of, Subject, throwError } from 'rxjs';
import { ArtifactService } from '../../../../../../../ng-swagger-gen/services/artifact.service';
import { ErrorHandler } from '../../../../../shared/units/error-handler';
import {
    BookmarkHealthService,
    BookmarkStatus,
} from '../../../../../shared/services/bookmark-health.service';
import { BookmarkKind } from '../../../../../shared/services/bookmark.service';
import { TagRedirectComponent } from './tag-redirect.component';

describe('TagRedirectComponent', () => {
    let fixture: ComponentFixture<TagRedirectComponent>;
    let params: BehaviorSubject<any>;
    let artifactService: jasmine.SpyObj<ArtifactService>;
    let health: jasmine.SpyObj<BookmarkHealthService>;
    let errorHandler: jasmine.SpyObj<ErrorHandler>;
    let router: jasmine.SpyObj<Router>;

    beforeEach(async () => {
        params = new BehaviorSubject(
            convertToParamMap({ repo: 'team/app', tag: 'latest' })
        );
        artifactService = jasmine.createSpyObj('ArtifactService', [
            'getArtifact',
        ]);
        health = jasmine.createSpyObj('BookmarkHealthService', ['report']);
        errorHandler = jasmine.createSpyObj('ErrorHandler', ['error']);
        router = jasmine.createSpyObj('Router', ['navigate']);
        await TestBed.configureTestingModule({
            declarations: [TagRedirectComponent],
            providers: [
                {
                    provide: ActivatedRoute,
                    useValue: {
                        paramMap: params,
                        snapshot: {
                            parent: {
                                params: { id: '1' },
                                data: { projectResolver: { name: 'library' } },
                            },
                        },
                    },
                },
                { provide: Router, useValue: router },
                { provide: ArtifactService, useValue: artifactService },
                { provide: BookmarkHealthService, useValue: health },
                { provide: ErrorHandler, useValue: errorHandler },
            ],
        }).compileComponents();
        fixture = TestBed.createComponent(TagRedirectComponent);
    });

    it('redirects to the digest the tag points to now', () => {
        artifactService.getArtifact.and.returnValue(
            of({ digest: 'sha256:abc' } as any)
        );
        fixture.detectChanges();
        expect(artifactService.getArtifact).toHaveBeenCalledWith(
            jasmine.objectContaining({
                projectName: 'library',
                repositoryName: 'team%252Fapp',
                reference: 'latest',
            })
        );
        expect(router.navigate).toHaveBeenCalledWith(
            [
                '/harbor/projects',
                '1',
                'repositories',
                'team/app',
                'artifacts-tab',
                'artifacts',
                'sha256:abc',
            ],
            { replaceUrl: true }
        );
        expect(health.report).toHaveBeenCalledWith(
            jasmine.objectContaining({ tag: 'latest' }),
            BookmarkStatus.OK
        );
    });

    it('reports a missing tag and falls back to the repository', () => {
        const error = new HttpErrorResponse({ status: 404 });
        artifactService.getArtifact.and.returnValue(throwError(error));
        fixture.detectChanges();
        expect(health.report).toHaveBeenCalledWith(
            jasmine.objectContaining({
                kind: BookmarkKind.TAG,
                projectId: 1,
                repoName: 'team/app',
                tag: 'latest',
            }),
            BookmarkStatus.MISSING
        );
        expect(errorHandler.error).toHaveBeenCalledWith(error);
        expect(router.navigate).toHaveBeenCalledWith(
            ['/harbor/projects', '1', 'repositories', 'team/app'],
            { replaceUrl: true }
        );
    });

    it('drops a pending lookup when the tag changes or the page closes', () => {
        const first = new Subject<any>();
        const second = new Subject<any>();
        artifactService.getArtifact.and.returnValues(first, second);
        fixture.detectChanges();
        params.next(convertToParamMap({ repo: 'team/app', tag: 'v2' }));
        first.next({ digest: 'sha256:old' });
        expect(router.navigate).not.toHaveBeenCalled();

        fixture.destroy();
        second.next({ digest: 'sha256:new' });
        expect(router.navigate).not.toHaveBeenCalled();
    });
});
