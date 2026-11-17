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
    ComponentFixture,
    TestBed,
    ComponentFixtureAutoDetect,
    fakeAsync,
    tick,
} from '@angular/core/testing';
import { TranslateService } from '@ngx-translate/core';
import { ListProjectComponent } from './list-project.component';
import { CUSTOM_ELEMENTS_SCHEMA, ChangeDetectorRef } from '@angular/core';
import { SessionService } from '../../../../shared/services/session.service';
import { AppConfigService } from '../../../../services/app-config.service';
import { SearchTriggerService } from '../../../../shared/components/global-search/search-trigger.service';
import { MessageHandlerService } from '../../../../shared/services/message-handler.service';
import { StatisticHandler } from '../statictics/statistic-handler.service';
import { of } from 'rxjs';
import { delay } from 'rxjs/operators';
import {
    ProjectService,
    UserPermissionService,
} from '../../../../shared/services';
import { OperationService } from '../../../../shared/components/operation/operation.service';
import { ConfirmationDialogService } from '../../../global-confirmation-dialog/confirmation-dialog.service';
import { SharedTestingModule } from '../../../../shared/shared.module';
describe('ListProjectComponent', () => {
    let component: ListProjectComponent;
    let fixture: ComponentFixture<ListProjectComponent>;
    const mockProjectService = {
        listProjects: () => {
            return of({
                body: [],
            }).pipe(delay(0));
        },
    };
    const mockSessionService = {
        getCurrentUser: () => {
            return false;
        },
    };
    const mockAppConfigService = {
        getConfig: () => {
            return {
                project_creation_restriction: '',
                with_chartmuseum: '',
            };
        },
    };
    const mockSearchTriggerService = {
        closeSearch: () => {},
    };
    const mockStatisticHandler = {
        handleError: () => {},
    };
    const mockMessageHandlerService = {
        refresh: () => {},
        showSuccess: () => {},
    };
    const mockConfirmationDialogService = {
        confirmationConfirm$: of({
            state: '',
            data: [],
        }),
    };
    const mockOperationService = {
        publishInfo$: () => {},
    };
    beforeEach(() => {
        TestBed.configureTestingModule({
            schemas: [CUSTOM_ELEMENTS_SCHEMA],
            imports: [SharedTestingModule],
            declarations: [ListProjectComponent],
            providers: [
                TranslateService,
                ChangeDetectorRef,
                { provide: ProjectService, useValue: mockProjectService },
                { provide: SessionService, useValue: mockSessionService },
                { provide: AppConfigService, useValue: mockAppConfigService },
                {
                    provide: SearchTriggerService,
                    useValue: mockSearchTriggerService,
                },
                {
                    provide: MessageHandlerService,
                    useValue: mockMessageHandlerService,
                },
                { provide: StatisticHandler, useValue: mockStatisticHandler },
                {
                    provide: ConfirmationDialogService,
                    useValue: mockConfirmationDialogService,
                },
                { provide: OperationService, useValue: mockOperationService },
                { provide: ComponentFixtureAutoDetect, useValue: true },
            ],
        }).compileComponents();
    });

    beforeEach(() => {
        fixture = TestBed.createComponent(ListProjectComponent);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });
    let originalTimeout;
    beforeEach(function () {
        originalTimeout = jasmine.DEFAULT_TIMEOUT_INTERVAL;
        jasmine.DEFAULT_TIMEOUT_INTERVAL = 100000;
    });

    afterEach(function () {
        jasmine.DEFAULT_TIMEOUT_INTERVAL = originalTimeout;
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    describe('canDelete', () => {
        const projects = (...ids: number[]) =>
            ids.map(id => ({ project_id: id, name: `p${id}` })) as any;
        let getPermission: jasmine.Spy;
        beforeEach(() => {
            getPermission = spyOn(
                TestBed.inject(UserPermissionService),
                'getPermission'
            );
        });

        it('is false with nothing selected', () => {
            component.selectedRow = [];
            component.selectionChanged();
            expect(component.canDelete).toBeFalse();
        });

        it('is true for a system admin without asking', () => {
            spyOn(mockSessionService, 'getCurrentUser').and.returnValue({
                has_admin_role: true,
            } as any);
            component.selectedRow = projects(1, 2);
            component.selectionChanged();
            expect(component.canDelete).toBeTrue();
            expect(getPermission).not.toHaveBeenCalled();
        });

        it('follows project:delete on every selected project', () => {
            getPermission.and.callFake((id: number) => of(id !== 2));
            component.selectedRow = projects(1);
            component.selectionChanged();
            expect(component.canDelete).toBeTrue();
            component.selectedRow = projects(1, 2);
            component.selectionChanged();
            expect(component.canDelete).toBeFalse();
        });

        it('ignores an answer for an earlier selection', fakeAsync(() => {
            getPermission.and.callFake((id: number) =>
                of(id === 1).pipe(delay(id === 1 ? 50 : 0))
            );
            component.selectedRow = projects(1);
            component.selectionChanged();
            component.selectedRow = projects(2);
            component.selectionChanged();
            tick(100);
            expect(component.canDelete).toBeFalse();
        }));
    });
});
