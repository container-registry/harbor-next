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
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { HttpResponse } from '@angular/common/http';
import { of } from 'rxjs';
import { SharedTestingModule } from '../../../../shared/shared.module';
import { ConfigService } from '../config.service';
import { BoolValueItem, Configuration } from '../config';
import { AuditlogService } from 'ng-swagger-gen/services/auditlog.service';
import { AuditLogConfigurationComponent } from './audit-log.component';

describe('AuditLogConfigurationComponent', () => {
    let component: AuditLogConfigurationComponent;
    let fixture: ComponentFixture<AuditLogConfigurationComponent>;
    let currentConfig: Configuration;
    const configService = {
        getConfig: () => currentConfig,
        setConfig: () => undefined,
        getOriginalConfig: () => new Configuration(),
        getLoadingConfigStatus: () => false,
        resetConfig: () => undefined,
        confirmUnsavedChanges: () => undefined,
        saveConfiguration: () => of(null),
        updateConfig: () => undefined,
    };
    const auditlogService = {
        listAuditLogEventTypesResponse: () =>
            of(
                new HttpResponse({
                    body: [
                        { event_type: 'login_user' },
                        { event_type: 'create_artifact' },
                        { event_type: 'pull_artifact' },
                        { event_type: 'create_member' },
                        { event_type: 'update_member' },
                        { event_type: 'delete_member' },
                        { event_type: 'create_project' },
                        { event_type: 'delete_project' },
                        { event_type: 'delete_repository' },
                    ],
                })
            ),
    };

    beforeEach(async () => {
        currentConfig = new Configuration();
        currentConfig.enable_commercial_audit_log_otlp = new BoolValueItem(
            true,
            true
        );
        await TestBed.configureTestingModule({
            imports: [SharedTestingModule],
            providers: [
                { provide: ConfigService, useValue: configService },
                { provide: AuditlogService, useValue: auditlogService },
            ],
            declarations: [AuditLogConfigurationComponent],
        }).compileComponents();
        fixture = TestBed.createComponent(AuditLogConfigurationComponent);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('shows Basic authentication fields conditionally', () => {
        currentConfig.audit_log_forward_otlp_authentication.value = 'basic';
        fixture.detectChanges();
        expect(
            fixture.nativeElement.querySelector('#auditLogForwardOTLPUsername')
        ).toBeTruthy();
        currentConfig.audit_log_forward_otlp_authentication.value = 'none';
        fixture.detectChanges();
        expect(
            fixture.nativeElement.querySelector('#auditLogForwardOTLPUsername')
        ).toBeFalsy();
    });

    it('hides only OTLP controls when the commercial feature is disabled', async () => {
        currentConfig.enable_commercial_audit_log_otlp.value = false;
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        const syslog = fixture.nativeElement.querySelector(
            '#auditLogForwardEndpoint'
        ) as HTMLInputElement;
        const otlp = fixture.nativeElement.querySelector(
            '#auditLogForwardOTLPEndpoint'
        );
        const matrix = fixture.nativeElement.querySelector(
            '.audit-log-matrix'
        ) as HTMLTableElement;

        expect(syslog.disabled).toBeFalse();
        expect(otlp).toBeNull();
        expect(
            fixture.nativeElement.querySelector(
                '#auditLogForwardOTLPAuthentication'
            )
        ).toBeNull();
        expect(matrix).toBeTruthy();
    });

    it('enables database skipping when an OTLP endpoint is configured', async () => {
        currentConfig.audit_log_forward_otlp_endpoint.value =
            'https://collector:4318';
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();
        const checkbox = fixture.nativeElement.querySelector(
            '#skipAuditLogDatabase'
        ) as HTMLInputElement;
        expect(checkbox.disabled).toBeFalse();
    });

    it('clears Basic credentials when authentication is disabled', () => {
        currentConfig.audit_log_forward_otlp_authentication.value = 'none';
        currentConfig.audit_log_forward_otlp_username.value = 'harbor';
        currentConfig.audit_log_forward_otlp_password.value = 'secret';
        component.onOTLPAuthenticationChange();
        expect(currentConfig.audit_log_forward_otlp_username.value).toBe('');
        expect(currentConfig.audit_log_forward_otlp_password.value).toBe('');
    });

    it('rejects scheme-less OTLP endpoints', () => {
        currentConfig.audit_log_forward_otlp_endpoint.value = 'collector:4318';
        fixture.detectChanges();
        expect(component.isValid()).toBeFalse();
    });

    it('groups audit log event types by resource', () => {
        expect(component.categories.map(category => category.id)).toEqual([
            'users',
            'artifacts',
            'projects',
            'repositories',
        ]);
        expect(component.categories[0].events.length).toBe(1);
        expect(
            component.categories
                .find(category => category.id === 'projects')
                ?.events.map(event => event.value)
        ).toEqual(['create_project', 'delete_project']);
        expect(
            component.categories.every(category =>
                category.events.every(event => !event.value.endsWith('_member'))
            )
        ).toBeTrue();
    });

    it('stores disabled event types when an event toggle is switched off', () => {
        component.toggleEventType('pull_artifact');

        expect(currentConfig.disabled_audit_log_event_types.value).toBe(
            'pull_artifact'
        );
        expect(component.hasEventType('pull_artifact')).toBeFalse();
        expect(component.hasChanges()).toBeTrue();
    });

    it('builds matrix actions from available event types', () => {
        expect(component.matrixActions).toEqual([
            'create',
            'update',
            'delete',
            'pull',
            'login',
        ]);
        expect(
            component.getMatrixEvent(component.categories[1], 'pull')?.value
        ).toBe('pull_artifact');
    });
});
