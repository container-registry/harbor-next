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
import { Component, Input, OnInit } from '@angular/core';
import { finalize } from 'rxjs/operators';
import { AdditionLink } from 'ng-swagger-gen/models';
import { AdditionsService } from '../additions.service';
import { ErrorHandler } from '../../../../../../shared/units/error-handler';
import { formatSize } from 'src/app/shared/units/utils';

export interface NpmVersion {
    version: string;
    created: string;
    yanked: boolean;
    size: number;
}

export interface DistTag {
    tag: string;
    version: string;
}

@Component({
    selector: 'hbr-artifact-versions',
    templateUrl: './versions.component.html',
    styleUrls: ['./versions.component.scss'],
})
export class ArtifactVersionsComponent implements OnInit {
    @Input() versionsLink: AdditionLink;
    versions: NpmVersion[] = [];
    distTags: DistTag[] = [];
    loading: boolean = false;

    constructor(
        private errorHandler: ErrorHandler,
        private additionsService: AdditionsService
    ) {}

    ngOnInit(): void {
        this.getVersions();
    }

    getVersions() {
        if (
            this.versionsLink &&
            !this.versionsLink.absolute &&
            this.versionsLink.href
        ) {
            this.loading = true;
            this.additionsService
                .getDetailByLink(this.versionsLink.href, false, false)
                .pipe(finalize(() => (this.loading = false)))
                .subscribe(
                    res => {
                        if (res && res.versions) {
                            this.versions = res.versions;
                        }
                        if (res && res.distTags) {
                            this.distTags = Object.keys(res.distTags).map(
                                tag => ({ tag, version: res.distTags[tag] })
                            );
                        }
                    },
                    error => {
                        this.errorHandler.error(error);
                    }
                );
        }
    }

    sizeTransform(size: number): string {
        return formatSize(size + '');
    }
}
