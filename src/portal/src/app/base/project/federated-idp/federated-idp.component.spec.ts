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
import { of } from 'rxjs';
import { ActivatedRoute } from '@angular/router';
import { FederatedIdpComponent } from './federated-idp.component';
import { UserPermissionService } from '../../../shared/services';
import { OperationService } from '../../../shared/components/operation/operation.service';
import { FederatedIdpService } from '../../../../../ng-swagger-gen/services/federated-idp.service';
import { FederatedIdp } from '../../../../../ng-swagger-gen/models/federated-idp';
import { delay } from 'rxjs/operators';
import { TranslateModule, TranslateService } from '@ngx-translate/core';
import { CommonModule } from '@angular/common';
import { ClarityModule } from '@clr/angular';
import { HttpClientTestingModule } from '@angular/common/http/testing';
import { RouterTestingModule } from '@angular/router/testing';
import { BrowserAnimationsModule } from '@angular/platform-browser/animations';
import { HarborDatetimePipe } from '../../../shared/pipes/harbor-datetime.pipe';
import { ErrorHandler } from '../../../shared/units/error-handler';

describe('FederatedIdpComponent', () => {
    let component: FederatedIdpComponent;
    let fixture: ComponentFixture<FederatedIdpComponent>;

    // Test data
    const idp1: FederatedIdp = {
        id: 1,
        name: 'test-idp-1',
        description: 'Test IDP 1',
        issuer: 'https://issuer1.example.com',
        supported_algorithms: ['RS256', 'ES256'],
        claims_supported: ['sub', 'email'],
        offline_validation: false,
        project_id: 1,
        creation_time: '2024-01-01T00:00:00Z',
        update_time: '2024-01-01T00:00:00Z',
    };

    const idp2: FederatedIdp = {
        id: 2,
        name: 'test-idp-2',
        description: 'Test IDP 2',
        issuer: 'https://issuer2.example.com',
        supported_algorithms: ['RS256'],
        claims_supported: ['sub'],
        offline_validation: true,
        project_id: 1,
        creation_time: '2024-01-02T00:00:00Z',
        update_time: '2024-01-02T00:00:00Z',
    };

    const idp3: FederatedIdp = {
        id: 3,
        name: 'test-idp-3',
        description: 'Test IDP 3',
        issuer: 'https://issuer3.example.com',
        supported_algorithms: ['ES256'],
        claims_supported: ['sub', 'name', 'email'],
        offline_validation: false,
        project_id: 1,
        creation_time: '2024-01-03T00:00:00Z',
        update_time: '2024-01-03T00:00:00Z',
    };

    const fakedFederatedIdpService = {
        ListFederatedIdps: jasmine
            .createSpy('ListFederatedIdps')
            .and.callFake(() => {
                return of([idp1, idp2, idp3]).pipe(delay(0));
            }),
        DeleteFederatedIdp: jasmine
            .createSpy('DeleteFederatedIdp')
            .and.callFake(() => {
                return of(null);
            }),
    };

    const fakedErrorHandler = {
        error: jasmine.createSpy('error'),
    };

    // Default admin permissions
    const mockUserPermissionService = {
        getPermission: jasmine
            .createSpy('getPermission')
            .and.callFake(
                (projectId: number, resource: string, action: string): any => {
                    // Default to admin permissions (all true)
                    return of(true);
                }
            ),
    };

    beforeEach(async () => {
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
                                    data: {
                                        projectResolver: {
                                            name: 'test-project',
                                        },
                                    },
                                },
                            },
                        },
                    },
                },
                { provide: ErrorHandler, useValue: fakedErrorHandler },
                OperationService,
                {
                    provide: UserPermissionService,
                    useValue: mockUserPermissionService,
                },
                {
                    provide: FederatedIdpService,
                    useValue: fakedFederatedIdpService,
                },
            ],
            declarations: [FederatedIdpComponent, HarborDatetimePipe],
        }).compileComponents();
    });

    beforeEach(() => {
        fixture = TestBed.createComponent(FederatedIdpComponent);
        component = fixture.componentInstance;
        // Reset spies
        fakedFederatedIdpService.ListFederatedIdps.calls.reset();
        fakedFederatedIdpService.DeleteFederatedIdp.calls.reset();
        mockUserPermissionService.getPermission.calls.reset();
        // Reset to admin permissions by default
        mockUserPermissionService.getPermission.and.callFake(() => of(true));
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should get project ID from route', () => {
        expect(component.projectId).toEqual(1);
    });

    it('should get project name from route resolver', () => {
        expect(component.projectName).toEqual('test-project');
    });

    describe('CRUD Operations', () => {
        it('should list federated IdPs on retrieve', async () => {
            component.retrieve();
            fixture.detectChanges();
            await fixture.whenStable();

            expect(
                fakedFederatedIdpService.ListFederatedIdps
            ).toHaveBeenCalled();
        });

        it('should build correct query for project-level IdPs', async () => {
            component.retrieve();
            fixture.detectChanges();
            await fixture.whenStable();

            const callArgs =
                fakedFederatedIdpService.ListFederatedIdps.calls.mostRecent()
                    .args[0];
            const decodedQuery = decodeURIComponent(callArgs.q);
            expect(decodedQuery).toContain('Level=project');
            expect(decodedQuery).toContain('ProjectID=1');
        });

        it('should search IdPs by name', async () => {
            component.doSearchTargets('test-idp');
            fixture.detectChanges();
            await fixture.whenStable();

            const callArgs =
                fakedFederatedIdpService.ListFederatedIdps.calls.mostRecent()
                    .args[0];
            const decodedQuery = decodeURIComponent(callArgs.q);
            expect(decodedQuery).toContain('name=~test-idp');
        });

        it('should refresh targets', async () => {
            component.targetName = 'some-search';
            component.page = 5;
            component.refreshTargets();
            fixture.detectChanges();
            await fixture.whenStable();

            expect(component.targetName).toEqual('');
            expect(component.page).toEqual(1);
            expect(
                fakedFederatedIdpService.ListFederatedIdps
            ).toHaveBeenCalled();
        });

        it('should get supported claims as comma-separated string', () => {
            const claims = ['sub', 'email', 'name'];
            const result = component.getSupportedClaims(claims);
            expect(result).toContain('sub');
            expect(result).toContain('email');
        });

        it('should truncate long claims string', () => {
            const claims = [
                'sub',
                'email',
                'name',
                'preferred_username',
                'groups',
            ];
            const result = component.getSupportedClaims(claims);
            expect(result.endsWith('…')).toBeTrue();
        });

        it('should return empty string for empty claims', () => {
            expect(component.getSupportedClaims([])).toEqual('');
            expect(component.getSupportedClaims(null)).toEqual('');
        });
    });

    describe('Role-Based Access Control - Project Admin / System Admin', () => {
        beforeEach(async () => {
            // Admin has all permissions
            mockUserPermissionService.getPermission.and.callFake(() =>
                of(true)
            );
            component.getPermissionsList();
            fixture.detectChanges();
            await fixture.whenStable();
        });

        it('should have all permissions for admin role', () => {
            expect(component.hasCreatePermission).toBeTrue();
            expect(component.hasUpdatePermission).toBeTrue();
            expect(component.hasDeletePermission).toBeTrue();
            expect(component.hasReadPermission).toBeTrue();
        });

        it('should enable create button for admin', async () => {
            fixture.detectChanges();
            await fixture.whenStable();
            const addButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#add');
            expect(addButton).toBeTruthy();
            expect(addButton.disabled).toBeFalse();
        });

        it('should enable edit button for admin when IdP selected', async () => {
            component.selectedRow = [idp1];
            fixture.detectChanges();
            await fixture.whenStable();
            const editButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#edit');
            expect(editButton).toBeTruthy();
            expect(editButton.disabled).toBeFalse();
        });

        it('should enable delete button for admin when IdP selected', async () => {
            component.selectedRow = [idp1];
            fixture.detectChanges();
            await fixture.whenStable();
            const deleteButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#delete');
            expect(deleteButton).toBeTruthy();
            expect(deleteButton.disabled).toBeFalse();
        });
    });

    describe('Role-Based Access Control - Maintainer Role', () => {
        beforeEach(async () => {
            // Maintainer: no permissions for federated-idp (admin-only feature)
            mockUserPermissionService.getPermission.and.callFake(() =>
                of(false)
            );
            component.getPermissionsList();
            fixture.detectChanges();
            await fixture.whenStable();
        });

        it('should have no permissions for maintainer role', () => {
            expect(component.hasCreatePermission).toBeFalse();
            expect(component.hasUpdatePermission).toBeFalse();
            expect(component.hasDeletePermission).toBeFalse();
            expect(component.hasReadPermission).toBeFalse();
        });

        it('should disable all action buttons for maintainer', async () => {
            component.selectedRow = [idp1];
            fixture.detectChanges();
            await fixture.whenStable();
            const addButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#add');
            const editButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#edit');
            const deleteButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#delete');
            expect(addButton.disabled).toBeTrue();
            expect(editButton.disabled).toBeTrue();
            expect(deleteButton.disabled).toBeTrue();
        });
    });

    describe('Role-Based Access Control - Developer Role', () => {
        beforeEach(async () => {
            // Developer: no permissions for federated-idp (admin-only feature)
            mockUserPermissionService.getPermission.and.callFake(() =>
                of(false)
            );
            component.getPermissionsList();
            fixture.detectChanges();
            await fixture.whenStable();
        });

        it('should have no permissions for developer role', () => {
            expect(component.hasCreatePermission).toBeFalse();
            expect(component.hasUpdatePermission).toBeFalse();
            expect(component.hasDeletePermission).toBeFalse();
            expect(component.hasReadPermission).toBeFalse();
        });

        it('should disable all action buttons for developer', async () => {
            component.selectedRow = [idp1];
            fixture.detectChanges();
            await fixture.whenStable();
            const addButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#add');
            const editButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#edit');
            const deleteButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#delete');
            expect(addButton.disabled).toBeTrue();
            expect(editButton.disabled).toBeTrue();
            expect(deleteButton.disabled).toBeTrue();
        });
    });

    describe('Role-Based Access Control - Guest Role', () => {
        beforeEach(async () => {
            // Guest: no permissions for federated-idp (admin-only feature)
            mockUserPermissionService.getPermission.and.callFake(() =>
                of(false)
            );
            component.getPermissionsList();
            fixture.detectChanges();
            await fixture.whenStable();
        });

        it('should have no permissions for guest role', () => {
            expect(component.hasCreatePermission).toBeFalse();
            expect(component.hasUpdatePermission).toBeFalse();
            expect(component.hasDeletePermission).toBeFalse();
            expect(component.hasReadPermission).toBeFalse();
        });

        it('should disable all action buttons for guest', async () => {
            component.selectedRow = [idp1];
            fixture.detectChanges();
            await fixture.whenStable();
            const addButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#add');
            const editButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#edit');
            const deleteButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#delete');
            expect(addButton.disabled).toBeTrue();
            expect(editButton.disabled).toBeTrue();
            expect(deleteButton.disabled).toBeTrue();
        });
    });

    describe('Role-Based Access Control - Limited Guest Role', () => {
        beforeEach(async () => {
            // Limited Guest: no permissions at all
            mockUserPermissionService.getPermission.and.callFake(() =>
                of(false)
            );
            component.getPermissionsList();
            fixture.detectChanges();
            await fixture.whenStable();
        });

        it('should have no permissions for limited guest role', () => {
            expect(component.hasCreatePermission).toBeFalse();
            expect(component.hasUpdatePermission).toBeFalse();
            expect(component.hasDeletePermission).toBeFalse();
            expect(component.hasReadPermission).toBeFalse();
        });

        it('should disable all action buttons for limited guest', async () => {
            component.selectedRow = [idp1];
            fixture.detectChanges();
            await fixture.whenStable();
            const addButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#add');
            const editButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#edit');
            const deleteButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#delete');
            expect(addButton.disabled).toBeTrue();
            expect(editButton.disabled).toBeTrue();
            expect(deleteButton.disabled).toBeTrue();
        });
    });

    describe('Permission Checks are called correctly', () => {
        it('should request all four permissions on init', async () => {
            fixture.detectChanges();
            await fixture.whenStable();

            expect(
                mockUserPermissionService.getPermission
            ).toHaveBeenCalledWith(1, 'federated-idp', 'create');
            expect(
                mockUserPermissionService.getPermission
            ).toHaveBeenCalledWith(1, 'federated-idp', 'update');
            expect(
                mockUserPermissionService.getPermission
            ).toHaveBeenCalledWith(1, 'federated-idp', 'delete');
            expect(
                mockUserPermissionService.getPermission
            ).toHaveBeenCalledWith(1, 'federated-idp', 'read');
        });
    });

    describe('Edit and Delete Operations', () => {
        beforeEach(async () => {
            // Admin permissions for edit/delete tests
            mockUserPermissionService.getPermission.and.callFake(() =>
                of(true)
            );
            component.getPermissionsList();
            fixture.detectChanges();
            await fixture.whenStable();
        });

        it('should allow editing when single IdP is selected', () => {
            component.selectedRow = [idp1];
            fixture.detectChanges();
            const editButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#edit');
            expect(editButton.disabled).toBeFalse();
        });

        it('should disable edit button when no IdP is selected', () => {
            component.selectedRow = [];
            fixture.detectChanges();
            const editButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#edit');
            expect(editButton.disabled).toBeTrue();
        });

        it('should disable edit button when multiple IdPs are selected', () => {
            component.selectedRow = [idp1, idp2];
            fixture.detectChanges();
            const editButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#edit');
            expect(editButton.disabled).toBeTrue();
        });

        it('should allow deleting when at least one IdP is selected', () => {
            component.selectedRow = [idp1];
            fixture.detectChanges();
            const deleteButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#delete');
            expect(deleteButton.disabled).toBeFalse();
        });

        it('should allow deleting multiple IdPs', () => {
            component.selectedRow = [idp1, idp2, idp3];
            fixture.detectChanges();
            const deleteButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#delete');
            expect(deleteButton.disabled).toBeFalse();
        });

        it('should disable delete button when no IdP is selected', () => {
            component.selectedRow = [];
            fixture.detectChanges();
            const deleteButton: HTMLButtonElement =
                fixture.nativeElement.querySelector('button#delete');
            expect(deleteButton.disabled).toBeTrue();
        });
    });
});
