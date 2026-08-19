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
import { Component, OnInit, ViewChild } from '@angular/core';
import { NgForm } from '@angular/forms';
import { finalize } from 'rxjs/operators';
import { AuditlogService } from 'ng-swagger-gen/services/auditlog.service';
import type { AuditLogEventType } from 'ng-swagger-gen/models/audit-log-event-type';
import { getChanges, isEmpty } from '../../../../shared/units/utils';
import { MessageHandlerService } from '../../../../shared/services/message-handler.service';
import { Configuration } from '../config';
import { ConfigService } from '../config.service';

interface AuditEvent {
    value: string;
    action: string;
}

interface AuditEventCategory {
    id: string;
    labelKey: string;
    icon: string;
    resourceTypes: string[];
    events: AuditEvent[];
}

const AUDIT_EVENT_CATEGORIES: Omit<AuditEventCategory, 'events'>[] = [
    {
        id: 'users',
        labelKey: 'AUDIT_LOG_CONFIG.USERS',
        icon: 'users',
        resourceTypes: ['user'],
    },
    {
        id: 'robots',
        labelKey: 'AUDIT_LOG_CONFIG.ROBOTS',
        icon: 'robot-head',
        resourceTypes: ['robot'],
    },
    {
        id: 'artifacts',
        labelKey: 'AUDIT_LOG_CONFIG.ARTIFACTS',
        icon: 'bundle',
        resourceTypes: ['artifact'],
    },
    {
        id: 'projects',
        labelKey: 'AUDIT_LOG_CONFIG.PROJECTS',
        icon: 'folder',
        resourceTypes: ['project'],
    },
    {
        id: 'configuration',
        labelKey: 'AUDIT_LOG_CONFIG.CONFIGURATION',
        icon: 'cog',
        resourceTypes: ['configuration'],
    },
    {
        id: 'repositories',
        labelKey: 'AUDIT_LOG_CONFIG.REPOSITORIES',
        icon: 'storage',
        resourceTypes: ['repository'],
    },
];

@Component({
    selector: 'app-audit-log-configuration',
    templateUrl: './audit-log.component.html',
    styleUrls: ['./audit-log.component.scss'],
})
export class AuditLogConfigurationComponent implements OnInit {
    onGoing = false;
    loading = false;
    logEventTypes: AuditEvent[] = [];
    categories: AuditEventCategory[] = [];
    matrixActions: string[] = [];

    @ViewChild('auditConfigForm') auditConfigForm: NgForm;

    get currentConfig(): Configuration {
        return this.conf.getConfig();
    }

    get otlpEnabled(): boolean {
        return (
            this.currentConfig.enable_commercial_audit_log_otlp?.value === true
        );
    }

    set currentConfig(config: Configuration) {
        this.conf.setConfig(config);
    }

    constructor(
        private conf: ConfigService,
        private auditlogService: AuditlogService,
        private messageHandler: MessageHandlerService
    ) {}

    ngOnInit(): void {
        this.conf.resetConfig();
        this.loadEventTypes();
    }

    loadEventTypes(): void {
        this.loading = true;
        this.auditlogService
            .listAuditLogEventTypesResponse()
            .pipe(finalize(() => (this.loading = false)))
            .subscribe({
                next: response => {
                    const eventTypes = response.body as AuditLogEventType[];
                    this.logEventTypes = eventTypes
                        .map(event => event.event_type)
                        .filter((eventType): eventType is string =>
                            Boolean(eventType)
                        )
                        .sort((first, second) =>
                            this.compareEventTypes(first, second)
                        )
                        .map(eventType => this.toAuditEvent(eventType));
                    this.categories = AUDIT_EVENT_CATEGORIES.map(category => ({
                        ...category,
                        events: this.logEventTypes.filter(event =>
                            category.resourceTypes.includes(
                                this.getResourceType(event.value)
                            )
                        ),
                    })).filter(category => category.events.length > 0);
                    this.matrixActions = this.getMatrixActions();
                },
                error: error => this.messageHandler.error(error),
            });
    }

    hasEventType(eventType: string): boolean {
        return !this.getDisabledEventTypes().has(eventType);
    }

    isEventSelectionDisabled(): boolean {
        return (
            this.disabled(this.currentConfig.disabled_audit_log_event_types) ||
            this.inProgress
        );
    }

    toggleEventType(eventType: string): void {
        if (this.isEventSelectionDisabled()) {
            return;
        }
        const disabled = this.getDisabledEventTypes();
        if (disabled.has(eventType)) {
            disabled.delete(eventType);
        } else {
            disabled.add(eventType);
        }
        this.setDisabledEventTypes(disabled);
    }

    getMatrixEvent(
        category: AuditEventCategory,
        action: string
    ): AuditEvent | undefined {
        return category.events.find(event => event.action === action);
    }

    getMatrixEventId(category: AuditEventCategory, action: string): string {
        return `audit-matrix-${category.id}-${action}`;
    }

    getChanges(): Record<string, unknown> {
        const allChanges = getChanges(
            this.conf.getOriginalConfig(),
            this.currentConfig
        );
        const changes: Record<string, unknown> = {};
        const otlpProperties = [
            'audit_log_forward_otlp_endpoint',
            'audit_log_forward_otlp_authentication',
            'audit_log_forward_otlp_username',
            'audit_log_forward_otlp_password',
        ];
        for (const property of Object.keys(allChanges ?? {})) {
            if (
                [
                    'audit_log_forward_endpoint',
                    ...(this.otlpEnabled ? otlpProperties : []),
                    'disabled_audit_log_event_types',
                    'skip_audit_log_database',
                ].includes(property)
            ) {
                changes[property] = allChanges[property];
            }
        }
        return changes;
    }

    hasChanges(): boolean {
        return !isEmpty(this.getChanges());
    }

    isValid(): boolean {
        return this.auditConfigForm?.valid && this.isOTLPEndpointValid();
    }

    isOTLPEndpointValid(): boolean {
        if (!this.otlpEnabled) {
            return true;
        }
        const endpoint =
            this.currentConfig.audit_log_forward_otlp_endpoint.value;
        if (!endpoint) {
            return true;
        }
        try {
            const parsed = new URL(endpoint);
            return (
                ['http:', 'https:'].includes(parsed.protocol) &&
                !!parsed.hostname &&
                !parsed.username &&
                !parsed.password &&
                (parsed.pathname === '' || parsed.pathname === '/') &&
                !parsed.search &&
                !parsed.hash
            );
        } catch {
            return false;
        }
    }

    get inProgress(): boolean {
        return (
            this.onGoing || this.loading || this.conf.getLoadingConfigStatus()
        );
    }

    disabled(property: { editable: boolean }): boolean {
        return !property?.editable;
    }

    otlpDisabled(property: { editable: boolean }): boolean {
        return !this.otlpEnabled || this.disabled(property);
    }

    onForwardEndpointInput(): void {
        if (
            !this.currentConfig.audit_log_forward_endpoint.value &&
            (!this.otlpEnabled ||
                !this.currentConfig.audit_log_forward_otlp_endpoint.value)
        ) {
            this.currentConfig.skip_audit_log_database.value = false;
        }
    }

    onOTLPAuthenticationChange(): void {
        if (
            this.currentConfig.audit_log_forward_otlp_authentication.value ===
            'none'
        ) {
            this.currentConfig.audit_log_forward_otlp_username.value = '';
            this.currentConfig.audit_log_forward_otlp_password.value = '';
        }
    }

    save(): void {
        const changes = this.getChanges();
        if (isEmpty(changes)) {
            return;
        }
        this.onGoing = true;
        this.conf
            .saveConfiguration(changes)
            .pipe(finalize(() => (this.onGoing = false)))
            .subscribe({
                next: () => {
                    this.conf.updateConfig();
                    this.messageHandler.info('CONFIG.SAVE_SUCCESS');
                },
                error: error => this.messageHandler.error(error),
            });
    }

    cancel(): void {
        if (this.hasChanges()) {
            this.conf.confirmUnsavedChanges(this.getChanges());
        }
    }

    private getResourceType(eventType: string): string {
        const separatorIndex = eventType.indexOf('_');
        return separatorIndex === -1
            ? eventType
            : eventType.slice(separatorIndex + 1);
    }

    private getDisabledEventTypes(): Set<string> {
        const value =
            this.currentConfig?.disabled_audit_log_event_types?.value ?? '';
        return new Set(value.split(',').filter(eventType => eventType));
    }

    private setDisabledEventTypes(disabled: Set<string>): void {
        this.currentConfig.disabled_audit_log_event_types.value =
            Array.from(disabled).join(',');
    }

    private toAuditEvent(eventType: string): AuditEvent {
        const action = eventType.split('_')[0];
        return {
            value: eventType,
            action,
        };
    }

    private getMatrixActions(): string[] {
        const actions = new Set(this.logEventTypes.map(event => event.action));
        return Array.from(actions).sort(
            (first, second) =>
                this.getActionOrder(first) - this.getActionOrder(second) ||
                first.localeCompare(second)
        );
    }

    private compareEventTypes(first: string, second: string): number {
        return (
            this.getActionOrder(first.split('_')[0]) -
                this.getActionOrder(second.split('_')[0]) ||
            first.localeCompare(second)
        );
    }

    private getActionOrder(action: string): number {
        const actionOrder = ['create', 'update', 'delete', 'pull'];
        const index = actionOrder.indexOf(action);
        return index === -1 ? actionOrder.length : index;
    }
}
