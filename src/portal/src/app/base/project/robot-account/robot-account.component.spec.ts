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
import { NO_ERRORS_SCHEMA } from '@angular/core';
import { of, Subscription, throwError } from 'rxjs';
import { ActivatedRoute } from '@angular/router';
import { MessageHandlerService } from '../../../shared/services/message-handler.service';
import { RobotAccountComponent } from './robot-account.component';
import { UserPermissionService } from '../../../shared/services';
import { OperationService } from '../../../shared/components/operation/operation.service';
import { RobotService } from '../../../../../ng-swagger-gen/services/robot.service';
import { FederatedIdpService } from '../../../../../ng-swagger-gen/services/federated-idp.service';
import { HttpHeaders, HttpResponse } from '@angular/common/http';
import { Robot } from '../../../../../ng-swagger-gen/models/robot';
import { FederatedIdp } from '../../../../../ng-swagger-gen/models/federated-idp';
import { delay } from 'rxjs/operators';
import {
    Action,
    PermissionsKinds,
    Resource,
} from '../../left-side-nav/system-robot-accounts/system-robot-util';
import { ConfirmationDialogService } from '../../global-confirmation-dialog/confirmation-dialog.service';
import { TranslateModule, TranslateService } from '@ngx-translate/core';
import { CommonModule } from '@angular/common';
import { ClarityModule } from '@clr/angular';
import { HttpClientTestingModule } from '@angular/common/http/testing';
import { RouterTestingModule } from '@angular/router/testing';
import { BrowserAnimationsModule } from '@angular/platform-browser/animations';
import { HarborDatetimePipe } from '../../../shared/pipes/harbor-datetime.pipe';
import { SysteminfoService } from '../../../../../ng-swagger-gen/services/systeminfo.service';

describe('RobotAccountComponent', () => {
    let component: RobotAccountComponent;
    let fixture: ComponentFixture<RobotAccountComponent>;
    const robot1: Robot = {
        id: 1,
        name: 'robot1',
        level: PermissionsKinds.PROJECT,
        disable: false,
        expires_at: (new Date().getTime() + 100000) % 1000,
        description: 'for test',
        secret: 'tthf54hfth4545dfgd5g454grd54gd54g',
        permissions: [
            {
                kind: PermissionsKinds.PROJECT,
                namespace: 'project1',
                access: [
                    {
                        resource: Resource.ARTIFACT,
                        action: Action.PUSH,
                    },
                ],
            },
        ],
    };
    const robot2: Robot = {
        id: 2,
        name: 'robot2',
        level: PermissionsKinds.PROJECT,
        disable: false,
        expires_at: (new Date().getTime() + 100000) % 1000,
        description: 'for test',
        secret: 'fsdf454654654fs6dfe',
        permissions: [
            {
                kind: PermissionsKinds.PROJECT,
                namespace: 'project2',
                access: [
                    {
                        resource: Resource.ARTIFACT,
                        action: Action.PUSH,
                    },
                ],
            },
        ],
    };
    const robot3: Robot = {
        id: 3,
        name: 'robot3',
        level: PermissionsKinds.PROJECT,
        disable: false,
        expires_at: (new Date().getTime() + 100000) % 1000,
        description: 'for test',
        secret: 'fsdg48454fse84',
        permissions: [
            {
                kind: PermissionsKinds.PROJECT,
                namespace: 'project3',
                access: [
                    {
                        resource: Resource.ARTIFACT,
                        action: Action.PUSH,
                    },
                ],
            },
        ],
    };
    // Federated IDP test data
    const systemIdp1: FederatedIdp = {
        id: 100,
        name: 'system-idp-1',
        description: 'System level IDP 1',
        issuer: 'https://system-idp-1.example.com',
        project_id: 0,
    };
    const systemIdp2: FederatedIdp = {
        id: 101,
        name: 'system-idp-2',
        description: 'System level IDP 2',
        issuer: 'https://system-idp-2.example.com',
        project_id: 0,
    };
    const projectIdp1: FederatedIdp = {
        id: 200,
        name: 'project-1-idp',
        description: 'Project 1 IDP',
        issuer: 'https://project-1-idp.example.com',
        project_id: 1, // This is the current project
    };
    const otherProjectIdp: FederatedIdp = {
        id: 300,
        name: 'other-project-idp',
        description: 'Other Project IDP',
        issuer: 'https://other-project-idp.example.com',
        project_id: 2, // Different project
    };

    const mockUserPermissionService = {
        getPermission() {
            return of(true);
        },
    };
    const fakedRobotService = {
        ListRobotResponse() {
            const res: HttpResponse<Array<Robot>> = new HttpResponse<
                Array<Robot>
            >({
                headers: new HttpHeaders({ 'x-total-count': '3' }),
                body: [robot1, robot2, robot3],
            });
            return of(res).pipe(delay(0));
        },
    };
    const fakedMessageHandlerService = {
        showSuccess() {},
        error() {},
    };
    let fakedFederatedIdpService: any;
    const fakedSysteminfoService = {
        getSystemInfo() {
            return of({
                enable_project_federated_idp: true,
            });
        },
    };
    beforeEach(async () => {
        // Initialize the federated IDP service mock with query-aware responses
        fakedFederatedIdpService = {
            ListFederatedIdps: jasmine
                .createSpy('ListFederatedIdps')
                .and.callFake((params: any) => {
                    const query = params?.q ? decodeURIComponent(params.q) : '';
                    // Return system IDPs for system-level query
                    if (query.includes('Level=system')) {
                        return of([systemIdp1, systemIdp2]);
                    }
                    // Return project IDPs for project-level query (project 1)
                    if (
                        query.includes('Level=project') &&
                        query.includes('ProjectID=1')
                    ) {
                        return of([projectIdp1]);
                    }
                    // Return other project IDPs for other project queries
                    if (
                        query.includes('Level=project') &&
                        query.includes('ProjectID=2')
                    ) {
                        return of([otherProjectIdp]);
                    }
                    // Default: return empty array
                    return of([]);
                }),
        };

        await TestBed.configureTestingModule({
            schemas: [NO_ERRORS_SCHEMA],
            imports: [
                TranslateModule.forRoot(),
                CommonModule,
                ClarityModule,
                HttpClientTestingModule,
                RouterTestingModule,
                BrowserAnimationsModule,
            ],
            providers: [
                TranslateService,
                {
                    provide: ActivatedRoute,
                    useValue: {
                        snapshot: {
                            parent: {
                                parent: {
                                    params: { id: 1 },
                                    data: null,
                                },
                            },
                        },
                    },
                },
                {
                    provide: MessageHandlerService,
                    useValue: fakedMessageHandlerService,
                },
                ConfirmationDialogService,
                OperationService,
                {
                    provide: UserPermissionService,
                    useValue: mockUserPermissionService,
                },
                { provide: RobotService, useValue: fakedRobotService },
                {
                    provide: FederatedIdpService,
                    useValue: fakedFederatedIdpService,
                },
                {
                    provide: SysteminfoService,
                    useValue: fakedSysteminfoService,
                },
            ],
            declarations: [RobotAccountComponent, HarborDatetimePipe],
        }).compileComponents();
    });

    beforeEach(() => {
        fixture = TestBed.createComponent(RobotAccountComponent);
        component = fixture.componentInstance;
        component.searchSub = new Subscription();
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });
    it('should render project robot list', async () => {
        fixture.autoDetectChanges();
        await fixture.whenStable();
        const rows = fixture.nativeElement.querySelectorAll('clr-dg-row');
        expect(rows.length).toEqual(3);
    });

    describe('Federated IDP Loading', () => {
        beforeEach(() => {
            // Reset spy call counts before each test in this describe block
            fakedFederatedIdpService.ListFederatedIdps.calls.reset();
        });

        it('should load both system and project IDPs when federated IDP is enabled', async () => {
            // Trigger the loading manually
            component.enableProjectFederatedIdp = true;
            component.loadFederatedIdps();
            fixture.detectChanges();
            await fixture.whenStable();

            // Verify ListFederatedIdps was called twice (once for system, once for project)
            expect(
                fakedFederatedIdpService.ListFederatedIdps
            ).toHaveBeenCalledTimes(2);

            // Verify the queries - should have system and project queries
            const calls =
                fakedFederatedIdpService.ListFederatedIdps.calls.allArgs();
            const queries = calls.map((call: any) =>
                decodeURIComponent(call[0]?.q || '')
            );
            expect(
                queries.some((q: string) => q.includes('Level=system'))
            ).toBeTrue();
            expect(
                queries.some(
                    (q: string) =>
                        q.includes('Level=project') && q.includes('ProjectID=1')
                )
            ).toBeTrue();
        });

        it('should include system IDPs in fedIdpMap', async () => {
            component.enableProjectFederatedIdp = true;
            component.loadFederatedIdps();
            fixture.detectChanges();
            await fixture.whenStable();

            // Verify system IDPs are in the map
            expect(component.fedIdpMap.has(100)).toBeTrue();
            expect(component.fedIdpMap.get(100)).toEqual('system-idp-1');
            expect(component.fedIdpMap.has(101)).toBeTrue();
            expect(component.fedIdpMap.get(101)).toEqual('system-idp-2');
        });

        it('should include project IDPs in fedIdpMap', async () => {
            component.enableProjectFederatedIdp = true;
            component.loadFederatedIdps();
            fixture.detectChanges();
            await fixture.whenStable();

            // Verify current project IDPs are in the map
            expect(component.fedIdpMap.has(200)).toBeTrue();
            expect(component.fedIdpMap.get(200)).toEqual('project-1-idp');
        });

        it('should NOT include other project IDPs in fedIdpMap', async () => {
            component.enableProjectFederatedIdp = true;
            component.loadFederatedIdps();
            fixture.detectChanges();
            await fixture.whenStable();

            // Verify other project IDPs are NOT in the map
            expect(component.fedIdpMap.has(300)).toBeFalse();
        });

        it('should have correct total count of IDPs (system + current project)', async () => {
            component.enableProjectFederatedIdp = true;
            component.loadFederatedIdps();
            fixture.detectChanges();
            await fixture.whenStable();

            // Should have 3 IDPs: 2 system + 1 project
            expect(component.fedIdpMap.size).toEqual(3);
        });

        it('should handle system IDP API error gracefully', async () => {
            // Reconfigure mock to throw error for system query
            fakedFederatedIdpService.ListFederatedIdps.and.callFake(
                (params: any) => {
                    const query = params?.q ? decodeURIComponent(params.q) : '';
                    if (query.includes('Level=system')) {
                        return throwError(() => new Error('System API error'));
                    }
                    if (
                        query.includes('Level=project') &&
                        query.includes('ProjectID=1')
                    ) {
                        return of([projectIdp1]);
                    }
                    return of([]);
                }
            );

            component.enableProjectFederatedIdp = true;
            component.loadFederatedIdps();
            fixture.detectChanges();
            await fixture.whenStable();

            // Should still have project IDP despite system API error
            expect(component.fedIdpMap.has(200)).toBeTrue();
            expect(component.fedIdpMap.get(200)).toEqual('project-1-idp');
        });

        it('should handle project IDP API error gracefully', async () => {
            // Reconfigure mock to throw error for project query
            fakedFederatedIdpService.ListFederatedIdps.and.callFake(
                (params: any) => {
                    const query = params?.q ? decodeURIComponent(params.q) : '';
                    if (query.includes('Level=system')) {
                        return of([systemIdp1, systemIdp2]);
                    }
                    if (
                        query.includes('Level=project') &&
                        query.includes('ProjectID=1')
                    ) {
                        return throwError(() => new Error('Project API error'));
                    }
                    return of([]);
                }
            );

            component.enableProjectFederatedIdp = true;
            component.loadFederatedIdps();
            fixture.detectChanges();
            await fixture.whenStable();

            // Should still have system IDPs despite project API error
            expect(component.fedIdpMap.has(100)).toBeTrue();
            expect(component.fedIdpMap.has(101)).toBeTrue();
        });

        it('should return correct IDP name via getFedIdpName for system IDP', async () => {
            component.enableProjectFederatedIdp = true;
            component.loadFederatedIdps();
            fixture.detectChanges();
            await fixture.whenStable();

            const robot: Robot = {
                id: 1,
                name: 'test-robot',
                federatedidp_id: 100, // system IDP
            };
            expect(component.getFedIdpName(robot)).toEqual('system-idp-1');
        });

        it('should return correct IDP name via getFedIdpName for project IDP', async () => {
            component.enableProjectFederatedIdp = true;
            component.loadFederatedIdps();
            fixture.detectChanges();
            await fixture.whenStable();

            const robot: Robot = {
                id: 1,
                name: 'test-robot',
                federatedidp_id: 200, // project IDP
            };
            expect(component.getFedIdpName(robot)).toEqual('project-1-idp');
        });

        it('should NOT load IDPs when user does not have FederatedIdp permission', async () => {
            fakedFederatedIdpService.ListFederatedIdps.calls.reset();

            // Setup: feature enabled but no permission
            component.configLoaded = true;
            component.permissionsLoaded = true;
            component.enableProjectFederatedIdp = true;
            component.hasFederatedIdpListPermission = false;

            component.maybeLoadFederatedIdps();
            fixture.detectChanges();
            await fixture.whenStable();

            // Should NOT have called the API
            expect(
                fakedFederatedIdpService.ListFederatedIdps
            ).not.toHaveBeenCalled();
        });

        it('should load IDPs when user has FederatedIdp permission and feature is enabled', async () => {
            fakedFederatedIdpService.ListFederatedIdps.calls.reset();

            // Setup: feature enabled AND has permission
            component.configLoaded = true;
            component.permissionsLoaded = true;
            component.enableProjectFederatedIdp = true;
            component.hasFederatedIdpListPermission = true;

            component.maybeLoadFederatedIdps();
            fixture.detectChanges();
            await fixture.whenStable();

            // Should have called the API
            expect(
                fakedFederatedIdpService.ListFederatedIdps
            ).toHaveBeenCalled();
        });
    });
});
