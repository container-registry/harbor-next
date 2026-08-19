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

interface CommercialFeature {
    configKey: string;
    name: string;
    description: string;
}

@Component({
    selector: 'commercial-features',
    templateUrl: './commercial-features.component.html',
})
export class CommercialFeaturesComponent implements OnInit {
    inProgress = false;
    readonly features: CommercialFeature[] = [
        {
            configKey: 'enable_commercial_branding',
            name: 'Branding',
            description: 'Customize the Harbor product and login experience.',
        },
        {
            configKey: 'enable_commercial_sftp_replication',
            name: 'SFTP Replication',
            description: 'Replicate artifacts to and from SFTP-backed storage.',
        },
        {
            configKey: 'enable_commercial_identity_providers',
            name: 'Identity Providers',
            description: 'Configure federated identity providers and their integrations.',
        },
        {
            configKey: 'enable_commercial_pgx_monitoring',
            name: 'PGX Monitoring',
            description: 'Collect PostgreSQL pool metrics with OpenTelemetry.',
        },
        {
            configKey: 'enable_commercial_aws_rds_iam_auth',
            name: 'AWS RDS IAM Authentication',
            description: 'Authenticate PostgreSQL connections with AWS RDS IAM.',
        },
        {
            configKey: 'enable_commercial_multi_format_artifacts',
            name: 'Multi-Format Artifacts',
            description: 'Enable commercial multi-format artifact repository support.',
        },
        {
            configKey: 'enable_commercial_audit_log_otlp',
            name: 'OTLP Audit Logging',
            description: 'Forward audit events to an OpenTelemetry endpoint.',
        },
    ];

    constructor(
        private configService: ConfigurationService,
        private config: ConfigService,
        private messageHandler: MessageHandlerService
    ) {}

    ngOnInit(): void {
        this.config.resetConfig();
    }

    featureConfig(feature: CommercialFeature): BoolValueItem {
        return this.config.getConfig()[feature.configKey] as BoolValueItem;
    }

    disabled(feature: CommercialFeature): boolean {
        const value = this.featureConfig(feature);
        return !value || !value.editable;
    }

    setEnabled(feature: CommercialFeature, enabled: boolean): void {
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
        const allChanges = getChanges(
            this.config.getOriginalConfig(),
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
