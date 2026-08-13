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
import { Inject, Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { map, catchError, shareReplay, tap } from 'rxjs/operators';
import { Observable, throwError as observableThrowError } from 'rxjs';
import { CustomStyle } from './theme';
import { DOCUMENT } from '@angular/common';
import { environment } from 'src/environments/environment';
import { BrandingConfig } from 'ng-swagger-gen/models';
import { SysteminfoService } from 'ng-swagger-gen/services';
@Injectable()
export class SkinableConfig {
    private customSkinData: CustomStyle;
    private BrandingInfo: BrandingConfig;
    constructor(
        private http: HttpClient,
        private systeminfoService: SysteminfoService,
        @Inject(DOCUMENT) private document: Document
    ) {}

    public getCustomFile(): Observable<any> {
        return this.http
            .get(`setting.json?buildTimeStamp=${environment?.buildTimestamp}`)
            .pipe(
                map(
                    response => (this.customSkinData = response as CustomStyle)
                ),
                catchError((error: any) => {
                    console.error('custom skin json file load failed');
                    return observableThrowError(error);
                })
            );
    }

    private brandingCache$: Observable<any> | null = null;
    public getBrandingInfo(refetch: boolean): Observable<any> {
        if (!refetch && this.brandingCache$) {
            return this.brandingCache$;
        }

        this.brandingCache$ = this.systeminfoService.getBrandingInfo().pipe(
            tap(branding => {
                this.BrandingInfo = branding;
            }),
            shareReplay(1),
            catchError(err => {
                this.brandingCache$ = null;
                return observableThrowError(err);
            })
        );
        return this.brandingCache$;
    }

    public getSkinConfig() {
        return this.customSkinData;
    }

    public getBrandingConfig() {
        return this.BrandingInfo;
    }

    public setTitleIcon(logoUrl: string) {
        if (!logoUrl) {
            return;
        }

        const titleIcon: HTMLLinkElement = this.document.querySelector('link');
        if (titleIcon) {
            titleIcon.href = logoUrl;
        }
    }
}
