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
import { Component, OnDestroy, OnInit } from '@angular/core';
import { ActivatedRoute, ParamMap, Router } from '@angular/router';
import { Observable, of, Subscription } from 'rxjs';
import { catchError, map, switchMap } from 'rxjs/operators';
import { ArtifactService as NewArtifactService } from '../../../../../../../ng-swagger-gen/services/artifact.service';
import { Project } from '../../../project';
import { dbEncodeURIComponent } from '../../../../../shared/units/utils';
import { ErrorHandler } from '../../../../../shared/units/error-handler';
import {
    BookmarkKind,
    BookmarkTarget,
} from '../../../../../shared/services/bookmark.service';
import {
    BookmarkHealthService,
    BookmarkStatus,
    statusFromError,
} from '../../../../../shared/services/bookmark-health.service';

// Tag bookmarks point here so they follow the tag when it moves to another digest.
@Component({
    selector: 'tag-redirect',
    template: `<div class="clr-row mt-3 center">
        <span class="spinner spinner-md"></span>
    </div>`,
})
export class TagRedirectComponent implements OnInit, OnDestroy {
    private sub: Subscription;

    constructor(
        private route: ActivatedRoute,
        private router: Router,
        private artifactService: NewArtifactService,
        private health: BookmarkHealthService,
        private errorHandler: ErrorHandler
    ) {}

    ngOnInit(): void {
        // switchMap drops a pending lookup when the route moves to another tag.
        this.sub = this.route.paramMap
            .pipe(switchMap(params => this.resolve(params)))
            .subscribe(commands =>
                this.router.navigate(commands, { replaceUrl: true })
            );
    }

    ngOnDestroy(): void {
        this.sub?.unsubscribe();
    }

    private resolve(params: ParamMap): Observable<any[]> {
        const projectId: string = this.route.snapshot.parent.params['id'];
        const project = <Project>(
            this.route.snapshot.parent.data['projectResolver']
        );
        const repoName = params.get('repo');
        const tag = params.get('tag');
        const target: BookmarkTarget = {
            kind: BookmarkKind.TAG,
            projectId: +projectId,
            projectName: project.name,
            repoName,
            tag,
            defaultLabel: '',
        };
        const repoRoute = [
            '/harbor/projects',
            projectId,
            'repositories',
            repoName,
        ];
        return this.artifactService
            .getArtifact({
                projectName: project.name,
                repositoryName: dbEncodeURIComponent(repoName),
                reference: tag,
                withTag: false,
                withScanOverview: false,
                withSbomOverview: false,
                withLabel: false,
                withAccessory: false,
            })
            .pipe(
                map(artifact => {
                    this.health.report(target, BookmarkStatus.OK);
                    return [
                        ...repoRoute,
                        'artifacts-tab',
                        'artifacts',
                        artifact.digest,
                    ];
                }),
                catchError(error => {
                    this.health.report(target, statusFromError(error));
                    this.errorHandler.error(error);
                    return of(repoRoute);
                })
            );
    }
}
