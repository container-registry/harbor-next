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
import { AppConfigService } from '../../../services/app-config.service';

/**
 * A tab of the Configuration page. A feature that brings its own settings
 * page calls registerConfigTab from its own file, next to its route, so no
 * two features edit the same template.
 */
export interface ConfigTab {
    // element id of the tab button
    id: string;
    route: string;
    // i18n key or plain text
    label: string;
    // hides the tab while it returns false
    visible?: (appConfig: AppConfigService) => boolean;
}

const tabs: ConfigTab[] = [
    { id: 'config-auth', route: 'auth', label: 'CONFIG.AUTH' },
    { id: 'config-security', route: 'security', label: 'CONFIG.SECURITY' },
    { id: 'config-system', route: 'setting', label: 'CONFIG.SYSTEM' },
    { id: 'config-audit-log', route: 'audit-log', label: 'CONFIG.AUDIT_LOG' },
    {
        id: 'config-features',
        route: 'features',
        label: 'CONFIG.FEATURES',
        visible: appConfig => appConfig.hasFeatures(),
    },
];

export function registerConfigTab(tab: ConfigTab): void {
    if (tabs.some(existing => existing.id === tab.id)) {
        throw new Error(`configuration tab ${tab.id} registered twice`);
    }
    tabs.push(tab);
}

export function configTabs(): ConfigTab[] {
    return [...tabs];
}
