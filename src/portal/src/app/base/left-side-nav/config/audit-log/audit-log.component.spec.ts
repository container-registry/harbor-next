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
import { Configuration } from '../config';
import { AuditlogService } from 'ng-swagger-gen/services/auditlog.service';
import { AuditLogConfigurationComponent } from './audit-log.component';

const DEFAULT_EVENT_TYPES = [
    { event_type: 'login_user' },
    { event_type: 'logout_user' },
    { event_type: 'create_artifact' },
    { event_type: 'delete_artifact' },
    { event_type: 'pull_artifact' },
    { event_type: 'create_member' },
    { event_type: 'update_member' },
    { event_type: 'delete_member' },
    { event_type: 'create_project' },
    { event_type: 'update_project' },
    { event_type: 'delete_project' },
    { event_type: 'delete_repository' },
    { event_type: 'create_robot' },
    { event_type: 'delete_robot' },
    { event_type: 'update_configuration' },
];

describe('AuditLogConfigurationComponent', () => {
    let component: AuditLogConfigurationComponent;
    let fixture: ComponentFixture<AuditLogConfigurationComponent>;
    let currentConfig: Configuration;
    // Rebuilt fresh in every beforeEach below (rather than declared as
    // module-level consts) so a spy's call history from one test — or an
    // overridden fake return value — can never leak into the next one.
    let configService: {
        getConfig: () => Configuration;
        setConfig: () => void;
        getOriginalConfig: () => Configuration;
        getLoadingConfigStatus: () => boolean;
        resetConfig: () => void;
        confirmUnsavedChanges: jasmine.Spy;
        saveConfiguration: jasmine.Spy;
        updateConfig: jasmine.Spy;
    };
    let auditlogService: {
        listAuditLogEventTypesResponse: jasmine.Spy;
    };

    beforeEach(async () => {
        currentConfig = new Configuration();
        configService = {
            getConfig: () => currentConfig,
            setConfig: () => undefined,
            getOriginalConfig: () => new Configuration(),
            getLoadingConfigStatus: () => false,
            resetConfig: () => undefined,
            confirmUnsavedChanges: jasmine.createSpy('confirmUnsavedChanges'),
            saveConfiguration: jasmine
                .createSpy('saveConfiguration')
                .and.returnValue(of(null)),
            updateConfig: jasmine.createSpy('updateConfig'),
        };
        auditlogService = {
            listAuditLogEventTypesResponse: jasmine
                .createSpy('listAuditLogEventTypesResponse')
                .and.returnValue(
                    of(new HttpResponse({ body: DEFAULT_EVENT_TYPES }))
                ),
        };
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

    it('creates the component', () => {
        expect(component).toBeTruthy();
    });

    it('groups audit log event types by resource, including project members', () => {
        expect(component.categories.map(category => category.id)).toEqual([
            'users',
            'members',
            'robots',
            'artifacts',
            'projects',
            'configuration',
            'repositories',
        ]);
        expect(
            component.categories
                .find(category => category.id === 'members')
                ?.events.map(event => event.value)
        ).toEqual(['create_member', 'update_member', 'delete_member']);
    });

    it('stores disabled event types when an event toggle is switched off', () => {
        component.toggleEventType('pull_artifact');

        expect(currentConfig.disabled_audit_log_event_types.value).toBe(
            'pull_artifact'
        );
        expect(component.hasEventType('pull_artifact')).toBeFalse();
        expect(component.hasChanges()).toBeTrue();
    });

    it('re-enables a previously disabled event type', () => {
        currentConfig.disabled_audit_log_event_types.value =
            'pull_artifact,delete_robot';
        component.toggleEventType('pull_artifact');

        expect(currentConfig.disabled_audit_log_event_types.value).toBe(
            'delete_robot'
        );
        expect(component.hasEventType('pull_artifact')).toBeTrue();
    });

    it('builds matrix actions from available event types', () => {
        expect(component.matrixActions).toEqual([
            'create',
            'update',
            'delete',
            'pull',
            'login',
            'logout',
        ]);
        expect(
            component.getMatrixEvent(component.categories[3], 'pull')?.value
        ).toBe('pull_artifact');
    });

    it('clears skip-database when the forward endpoint is cleared', () => {
        currentConfig.skip_audit_log_database.value = true;
        currentConfig.audit_log_forward_endpoint.value = '';
        component.onForwardEndpointInput();
        expect(currentConfig.skip_audit_log_database.value).toBeFalse();
    });

    it('only sends audit-log-scoped keys on save', () => {
        currentConfig.audit_log_forward_endpoint.value = 'harbor-log:10514';
        expect(Object.keys(component.getChanges())).toEqual([
            'audit_log_forward_endpoint',
        ]);
    });

    it('save() sends only the changed audit-log keys and refreshes the config', () => {
        currentConfig.audit_log_forward_endpoint.value = 'harbor-log:10514';
        component.save();
        expect(configService.saveConfiguration).toHaveBeenCalledWith({
            audit_log_forward_endpoint: 'harbor-log:10514',
        });
        expect(configService.updateConfig).toHaveBeenCalled();
    });

    it('save() is a no-op when nothing changed', () => {
        component.save();
        expect(configService.saveConfiguration).not.toHaveBeenCalled();
    });

    it('cancel() asks for confirmation only when there are changes', () => {
        component.cancel();
        expect(configService.confirmUnsavedChanges).not.toHaveBeenCalled();

        currentConfig.skip_audit_log_database.value = true;
        currentConfig.audit_log_forward_endpoint.value = 'harbor-log:10514';
        component.cancel();
        expect(configService.confirmUnsavedChanges).toHaveBeenCalledWith({
            audit_log_forward_endpoint: 'harbor-log:10514',
            skip_audit_log_database: true,
        });
    });

    describe('rendered UI', () => {
        function matrixCheckbox(
            categoryId: string,
            action: string
        ): HTMLInputElement | null {
            return fixture.nativeElement.querySelector(
                `#audit-matrix-${categoryId}-${action}`
            );
        }

        it('renders one matrix row per category and checks only enabled event types', () => {
            const rows = fixture.nativeElement.querySelectorAll(
                '.audit-log-matrix tbody tr'
            );
            expect(rows.length).toBe(component.categories.length);

            const pullArtifact = matrixCheckbox('artifacts', 'pull');
            expect(pullArtifact).toBeTruthy();
            expect(pullArtifact.checked).toBeTrue();

            currentConfig.disabled_audit_log_event_types.value =
                'pull_artifact';
            fixture.detectChanges();
            expect(matrixCheckbox('artifacts', 'pull').checked).toBeFalse();
        });

        it('does not render a cell for an action a category has no event for', () => {
            // 'login'/'logout' only apply to the users category
            expect(matrixCheckbox('artifacts', 'login')).toBeFalsy();
            expect(matrixCheckbox('users', 'login')).toBeTruthy();
        });

        it('labels each matrix checkbox with its resource and action for screen readers', () => {
            const checkbox = matrixCheckbox('artifacts', 'pull');
            expect(checkbox.getAttribute('aria-label')).toContain('Pull');
        });

        it('toggling a matrix checkbox in the DOM disables that event type', () => {
            const checkbox = matrixCheckbox('artifacts', 'pull');
            checkbox.checked = false;
            checkbox.dispatchEvent(new Event('change'));
            fixture.detectChanges();

            expect(currentConfig.disabled_audit_log_event_types.value).toBe(
                'pull_artifact'
            );
        });

        it('disables every matrix checkbox once event selection is disabled', () => {
            currentConfig.disabled_audit_log_event_types.editable = false;
            fixture.detectChanges();

            const checkboxes = fixture.nativeElement.querySelectorAll(
                '.audit-log-matrix input[type=checkbox]'
            );
            expect(checkboxes.length).toBeGreaterThan(0);
            checkboxes.forEach((box: HTMLInputElement) =>
                expect(box.disabled).toBeTrue()
            );
        });

        it('shows an empty-state alert instead of the matrix when there are no event types', async () => {
            // Reuses the already-configured TestBed module and just swaps
            // what the (spy-backed) AuditlogService resolves before
            // creating a second component instance, instead of tearing
            // down/reconfiguring TestBed mid-test.
            auditlogService.listAuditLogEventTypesResponse.and.returnValue(
                of(new HttpResponse({ body: [] }))
            );
            const emptyFixture = TestBed.createComponent(
                AuditLogConfigurationComponent
            );
            emptyFixture.detectChanges();
            await emptyFixture.whenStable();
            emptyFixture.detectChanges();

            expect(
                emptyFixture.nativeElement.querySelector('.audit-log-matrix')
            ).toBeFalsy();
            expect(
                emptyFixture.nativeElement.querySelector('clr-alert')
            ).toBeTruthy();
        });

        it('types into the forward endpoint field and reflects it on the model', () => {
            const input: HTMLInputElement = fixture.nativeElement.querySelector(
                '#auditLogForwardEndpoint'
            );
            input.value = 'harbor-log:10514';
            input.dispatchEvent(new Event('input'));
            fixture.detectChanges();

            expect(currentConfig.audit_log_forward_endpoint.value).toBe(
                'harbor-log:10514'
            );
        });

        it('disables skip-database until a forward endpoint is set', async () => {
            currentConfig.audit_log_forward_endpoint.value = '';
            fixture.detectChanges();
            expect(
                (
                    fixture.nativeElement.querySelector(
                        '#skipAuditLogDatabase'
                    ) as HTMLInputElement
                ).disabled
            ).toBeTrue();

            currentConfig.audit_log_forward_endpoint.value = 'harbor-log:10514';
            fixture.detectChanges();
            // Clarity's clrCheckbox control-value-accessor propagates a
            // disabled -> enabled transition asynchronously; a second sync
            // detectChanges() isn't enough, so flush before asserting.
            await fixture.whenStable();
            fixture.detectChanges();
            expect(
                (
                    fixture.nativeElement.querySelector(
                        '#skipAuditLogDatabase'
                    ) as HTMLInputElement
                ).disabled
            ).toBeFalse();
        });

        it('disables Save/Cancel until there are changes, and enables them once there are', () => {
            expect(
                (
                    fixture.nativeElement.querySelector(
                        '#config_audit_log_save'
                    ) as HTMLButtonElement
                ).disabled
            ).toBeTrue();
            expect(
                (
                    fixture.nativeElement.querySelector(
                        '#config_audit_log_cancel'
                    ) as HTMLButtonElement
                ).disabled
            ).toBeTrue();

            currentConfig.audit_log_forward_endpoint.value = 'harbor-log:10514';
            fixture.detectChanges();
            expect(
                (
                    fixture.nativeElement.querySelector(
                        '#config_audit_log_save'
                    ) as HTMLButtonElement
                ).disabled
            ).toBeFalse();
            expect(
                (
                    fixture.nativeElement.querySelector(
                        '#config_audit_log_cancel'
                    ) as HTMLButtonElement
                ).disabled
            ).toBeFalse();
        });

        it('clicking Save invokes save() and persists the change', () => {
            currentConfig.audit_log_forward_endpoint.value = 'harbor-log:10514';
            fixture.detectChanges();
            const saveBtn: HTMLButtonElement =
                fixture.nativeElement.querySelector('#config_audit_log_save');
            saveBtn.click();

            expect(configService.saveConfiguration).toHaveBeenCalledWith({
                audit_log_forward_endpoint: 'harbor-log:10514',
            });
        });

        it('clicking Cancel invokes cancel() and asks for confirmation', () => {
            currentConfig.audit_log_forward_endpoint.value = 'harbor-log:10514';
            fixture.detectChanges();
            const cancelBtn: HTMLButtonElement =
                fixture.nativeElement.querySelector('#config_audit_log_cancel');
            cancelBtn.click();

            expect(configService.confirmUnsavedChanges).toHaveBeenCalledWith({
                audit_log_forward_endpoint: 'harbor-log:10514',
            });
        });
    });
});
