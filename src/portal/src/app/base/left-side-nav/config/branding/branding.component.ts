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
import {
    AfterViewChecked,
    ChangeDetectorRef,
    Component,
    OnInit,
    ViewChild,
} from '@angular/core';
import { NgForm } from '@angular/forms';
import { Configuration } from '../config';
import { ConfigService } from '../config.service';
import { AppConfigService } from '../../../../services/app-config.service';
import { MessageHandlerService } from '../../../../shared/services/message-handler.service';
import { SysteminfoService } from 'ng-swagger-gen/services';
import { BrandingConfig } from 'ng-swagger-gen/models';
import { SkinableConfig } from 'src/app/services/skinable-config.service';

@Component({
    selector: 'branding',
    templateUrl: './branding.component.html',
    styleUrls: ['./branding.component.scss'],
})
export class BrandingComponent implements OnInit, AfterViewChecked {
    onGoing = false;

    get currentConfig(): Configuration {
        return this.conf.getConfig();
    }

    set currentConfig(cfg: Configuration) {
        this.conf.setConfig(cfg);
    }

    productName: string = '';
    productLogo: string = '';
    productIntroduction: string = '';
    loginTitle: string = '';
    loginBgImg: string = '';
    headerBgColorLight: string = '';
    headerBgColorDark: string = '';

    productNameCopy: string = '';
    productLogoCopy: string = '';
    productIntroductionCopy: string = '';
    loginTitleCopy: string = '';
    loginBgImgCopy: string = '';
    headerBgColorLightCopy: string = '';
    headerBgColorDarkCopy: string = '';

    @ViewChild('brandingForm') brandingForm: NgForm;

    constructor(
        private appConfigService: AppConfigService,
        private errorHandler: MessageHandlerService,
        private conf: ConfigService,
        private changeDetectorRef: ChangeDetectorRef,
        private systeminfoService: SysteminfoService,
        private skinableConfig: SkinableConfig
    ) {}

    ngOnInit() {
        this.getAndSetBrandingConfig();
    }

    ngAfterViewChecked() {
        this.changeDetectorRef.detectChanges();
    }

    getAndSetBrandingConfig() {
        this.skinableConfig.getBrandingInfo(true).subscribe({
            next: branding => {
                if (branding) {
                    this.productName = branding?.product?.name || '';
                    this.productLogo = branding?.product?.logo || '';
                    this.productIntroduction =
                        branding?.product?.introduction || '';
                    this.loginTitle = branding?.loginTitle || '';
                    this.loginBgImg = branding?.loginBgImg || '';
                    this.headerBgColorLight =
                        branding?.headerBgColor?.lightMode || '';
                    this.headerBgColorDark =
                        branding?.headerBgColor?.darkMode || '';

                    this.productNameCopy = this.productName;
                    this.productLogoCopy = this.productLogo;
                    this.productIntroductionCopy = this.productIntroduction;
                    this.loginTitleCopy = this.loginTitle;
                    this.loginBgImgCopy = this.loginBgImg;
                    this.headerBgColorLightCopy = this.headerBgColorLight;
                    this.headerBgColorDarkCopy = this.headerBgColorDark;
                }
            },
            error: () => {
                this.errorHandler.error('Failed to load branding config');
            },
        });
    }

    public isValid(): boolean {
        return this.brandingForm && this.brandingForm.valid;
    }

    public hasChanges(): boolean {
        return this.hasBrandingChanged();
    }

    hasBrandingChanged(): boolean {
        return (
            this.productName !== this.productNameCopy ||
            this.productLogo !== this.productLogoCopy ||
            this.productIntroduction !== this.productIntroductionCopy ||
            this.loginTitle !== this.loginTitleCopy ||
            this.loginBgImg !== this.loginBgImgCopy ||
            this.headerBgColorLight !== this.headerBgColorLightCopy ||
            this.headerBgColorDark !== this.headerBgColorDarkCopy
        );
    }

    public save(): void {
        if (!this.hasChanges()) {
            return;
        }

        const config: BrandingConfig = {
            headerBgColor: {
                darkMode: this.headerBgColorDark,
                lightMode: this.headerBgColorLight,
            },
            loginBgImg: this.loginBgImg,
            loginTitle: this.loginTitle,
            product: {
                name: this.productName,
                logo: this.productLogo,
                introduction: this.productIntroduction,
            },
        };

        this.onGoing = true;
        this.systeminfoService.updateBrandingInfo({ brandingConfig: config }).subscribe({
            next: () => {
                this.onGoing = false;
                this.getAndSetBrandingConfig();
                this.appConfigService.load().subscribe();
                this.errorHandler.info('CONFIG.SAVE_SUCCESS');
            },
            error: error => {
                this.onGoing = false;
                this.errorHandler.error(error);
            },
        });
    }

    public get inProgress(): boolean {
        return this.onGoing || this.conf.getLoadingConfigStatus();
    }

    public cancel(): void {
        if (this.hasChanges()) {
            this.productName = this.productNameCopy;
            this.productLogo = this.productLogoCopy;
            this.productIntroduction = this.productIntroductionCopy;
            this.loginTitle = this.loginTitleCopy;
            this.loginBgImg = this.loginBgImgCopy;
            this.headerBgColorLight = this.headerBgColorLightCopy;
            this.headerBgColorDark = this.headerBgColorDarkCopy;
        }
    }
}
