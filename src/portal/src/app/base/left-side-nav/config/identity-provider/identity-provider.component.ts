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
import { AppConfigService } from '../../../../services/app-config.service';
import { ConfigurationService } from '../../../../services/config.service';
import { MessageHandlerService } from '../../../../shared/services/message-handler.service';
import { BoolValueItem, Configuration } from '../config';
import { ConfigService } from '../config.service';

const PROJECT_IDP_CONFIG_KEY =
    'enable_project_federated_idp' satisfies keyof Configuration;

@Component({
    selector: 'identity-provider-configuration',
    templateUrl: './identity-provider.component.html',
    styleUrls: ['./identity-provider.component.scss'],
})
export class IdentityProviderConfigurationComponent implements OnInit {
    saving = false;

    constructor(
        private configService: ConfigurationService,
        private appConfigService: AppConfigService,
        private config: ConfigService,
        private messageHandler: MessageHandlerService
    ) {}

    ngOnInit(): void {
        this.config.resetConfig();
    }

    get projectIdentityProviders(): BoolValueItem | undefined {
        return this.config.getConfig()[PROJECT_IDP_CONFIG_KEY];
    }

    setEnabled(enabled: boolean): void {
        const setting = this.projectIdentityProviders;
        if (setting?.editable) {
            setting.value = enabled;
        }
    }

    hasChanges(): boolean {
        const current = this.projectIdentityProviders;
        const original =
            this.config.getOriginalConfig()?.[PROJECT_IDP_CONFIG_KEY];
        return !!current && !!original && current.value !== original.value;
    }

    save(): void {
        const setting = this.projectIdentityProviders;
        if (!setting?.editable || !this.hasChanges()) {
            return;
        }

        this.saving = true;
        this.configService
            .saveConfiguration({
                [PROJECT_IDP_CONFIG_KEY]: setting.value,
            })
            .subscribe({
                next: () => {
                    this.saving = false;
                    this.config.updateConfig();
                    this.appConfigService.load().subscribe({
                        error: error => this.messageHandler.handleError(error),
                    });
                    this.messageHandler.showSuccess('CONFIG.SAVE_SUCCESS');
                },
                error: error => {
                    this.saving = false;
                    this.messageHandler.handleError(error);
                },
            });
    }

    cancel(): void {
        this.config.resetConfig();
    }
}
