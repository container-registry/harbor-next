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
import { Component, OnInit } from '@angular/core';
import { TranslateService } from '@ngx-translate/core';
import { ConfigService } from './config.service';
import { AppConfigService } from '../../../services/app-config.service';
import { ConfigTab, configTabs } from './config-tabs';

@Component({
    selector: 'config',
    templateUrl: 'config.component.html',
    styleUrls: ['config.component.scss'],
})
export class ConfigurationComponent implements OnInit {
    get inProgress(): boolean {
        return this.conf.getLoadingConfigStatus();
    }

    constructor(
        private conf: ConfigService,
        private appConfigService: AppConfigService,
        private translate: TranslateService
    ) {}

    // Tabs come from upstream and from features that add their own, so list
    // them by translated label; DOM order then matches what is shown.
    get tabs(): ConfigTab[] {
        const label = (tab: ConfigTab) => this.translate.instant(tab.label);
        return configTabs()
            .filter(tab => !tab.visible || tab.visible(this.appConfigService))
            .sort((a, b) => label(a).localeCompare(label(b)));
    }

    trackTab(_: number, tab: ConfigTab): string {
        return tab.id;
    }

    ngOnInit(): void {
        // Refresh config every time when entering the configuration page
        this.conf.updateConfig();
    }
}
