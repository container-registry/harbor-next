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
    Component,
    ElementRef,
    OnInit,
    ViewChild,
} from '@angular/core';
import { ConfigService } from './config.service';
import { AppConfigService } from '../../../services/app-config.service';

@Component({
    selector: 'config',
    templateUrl: 'config.component.html',
    styleUrls: ['config.component.scss'],
})
export class ConfigurationComponent implements OnInit, AfterViewChecked {
    @ViewChild('tabs', { static: true }) tabs: ElementRef<HTMLElement>;

    get inProgress(): boolean {
        return this.conf.getLoadingConfigStatus();
    }

    constructor(
        private conf: ConfigService,
        private appConfigService: AppConfigService
    ) {}

    get hasFeatures(): boolean {
        return this.appConfigService.hasFeatures();
    }

    // Tabs that belong to one feature show only while it is enabled.
    featureEnabled(key: string): boolean {
        return this.appConfigService.featureEnabled(key);
    }

    ngOnInit(): void {
        // Refresh config every time when entering the configuration page
        this.conf.updateConfig();
    }

    // Tabs come from upstream and from optional features, so sort them by
    // their translated label instead of relying on where each was added.
    ngAfterViewChecked(): void {
        const items = Array.from(
            this.tabs.nativeElement.children
        ) as HTMLElement[];
        items
            .sort((a, b) =>
                a.textContent.trim().localeCompare(b.textContent.trim())
            )
            .forEach((item, index) => (item.style.order = String(index)));
    }
}
