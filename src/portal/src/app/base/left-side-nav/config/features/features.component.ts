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
import { ConfigurationService } from '../../../../services/config.service';
import { MessageHandlerService } from '../../../../shared/services/message-handler.service';
import { getChanges, isEmpty } from '../../../../shared/units/utils';
import { BoolValueItem, Configuration } from '../config';
import { ConfigService } from '../config.service';
import { FeatureToggle } from './feature-toggle';
import { OPTIONAL_FEATURES, featureToggles } from './optional-features';

@Component({
    selector: 'config-features',
    templateUrl: './features.component.html',
    styleUrls: ['./features.component.scss'],
})
export class FeaturesComponent implements OnInit {
    inProgress = false;
    readonly optionalFeatures: FeatureToggle[] = OPTIONAL_FEATURES;
    readonly features: FeatureToggle[] = featureToggles();

    constructor(
        private configService: ConfigurationService,
        private config: ConfigService,
        private messageHandler: MessageHandlerService
    ) {}

    ngOnInit(): void {
        this.config.resetConfig();
    }

    featureConfig(feature: FeatureToggle): BoolValueItem {
        return this.config.getConfig()[feature.configKey] as BoolValueItem;
    }

    disabled(feature: FeatureToggle): boolean {
        const value = this.featureConfig(feature);
        return !value || !value.editable;
    }

    lockedBy(feature: FeatureToggle): string {
        if (!this.featureConfig(feature) || !this.disabled(feature)) {
            return '';
        }
        // CONFIG_OVERWRITE_JSON locks every setting, not only this one.
        if (this.config.getConfig().token_expiration?.editable === false) {
            return 'CONFIG_OVERWRITE_JSON';
        }
        return feature.envKey;
    }

    setEnabled(feature: FeatureToggle, enabled: boolean): void {
        const value = this.featureConfig(feature);
        if (value) {
            value.value = enabled;
        }
    }

    hasChanges(): boolean {
        return !isEmpty(this.changes());
    }

    save(): void {
        const changes = this.changes();
        if (isEmpty(changes)) {
            return;
        }
        this.inProgress = true;
        this.configService.saveConfiguration(changes).subscribe({
            next: () => {
                this.inProgress = false;
                this.config.updateConfig();
                this.messageHandler.showSuccess('CONFIG.SAVE_SUCCESS');
            },
            error: error => {
                this.inProgress = false;
                this.messageHandler.handleError(error);
            },
        });
    }

    cancel(): void {
        this.config.resetConfig();
    }

    private changes(): { [key: string]: boolean } {
        const original = this.config.getOriginalConfig();
        if (!original) {
            return {};
        }
        const allChanges = getChanges(
            original,
            this.config.getConfig() as Configuration
        );
        return this.features.reduce((changes, feature) => {
            if (allChanges[feature.configKey] !== undefined) {
                changes[feature.configKey] = allChanges[feature.configKey];
            }
            return changes;
        }, {} as { [key: string]: boolean });
    }
}
