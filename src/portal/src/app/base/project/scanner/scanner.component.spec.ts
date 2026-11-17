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
import { of } from 'rxjs';
import { TranslateService } from '@ngx-translate/core';
import { MessageHandlerService } from '../../../shared/services/message-handler.service';
import { ScannerComponent } from './scanner.component';
import { SharedTestingModule } from '../../../shared/shared.module';
import { ActivatedRoute, Router } from '@angular/router';
import { CommonRoutes } from '../../../shared/entities/shared.const';
import { Scanner } from '../../left-side-nav/interrogation-services/scanner/scanner';
import { ErrorHandler } from '../../../shared/units/error-handler';
import { ProjectService } from '../../../../../ng-swagger-gen/services/project.service';
import { HttpHeaders, HttpResponse } from '@angular/common/http';
import { Registry } from '../../../../../ng-swagger-gen/models/registry';

describe('ScannerComponent', () => {
    const mockScanner1: Scanner = {
        uuid: 'abc',
        name: 'test1',
        description: 'just a sample',
        version: '1.0.0',
        url: 'http://168.0.0.1',
        health: 'healthy',
    };
    const mockScanner2: Scanner = {
        uuid: 'def',
        name: 'test2',
        description: 'just a sample',
        version: '2.0.0',
        url: 'http://168.0.0.2',
        health: 'healthy',
    };
    let component: ScannerComponent;
    let fixture: ComponentFixture<ScannerComponent>;
    let fakedProjectService = {
        getScannerOfProject() {
            return of(mockScanner1);
        },
        listScannerCandidatesOfProject() {
            return of([mockScanner1, mockScanner2]);
        },
        listScannerCandidatesOfProjectResponse() {
            const response: HttpResponse<Array<Registry>> = new HttpResponse<
                Array<Registry>
            >({
                headers: new HttpHeaders({
                    'x-total-count': [
                        mockScanner1,
                        mockScanner2,
                    ].length.toString(),
                }),
                body: [mockScanner1, mockScanner2],
            });
            return of(response);
        },
        setScannerOfProject() {
            return of(true);
        },
    };
    let fakedRoute = {
        snapshot: {
            parent: {
                parent: {
                    params: {
                        id: 1,
                    },
                },
            },
        },
    };
    beforeEach(() => {
        TestBed.configureTestingModule({
            imports: [SharedTestingModule],
            declarations: [ScannerComponent],
            providers: [
                TranslateService,
                MessageHandlerService,
                ErrorHandler,
                { provide: ActivatedRoute, useValue: fakedRoute },
                { provide: ProjectService, useValue: fakedProjectService },
            ],
        }).compileComponents();
    });
    beforeEach(() => {
        fixture = TestBed.createComponent(ScannerComponent);
        component = fixture.componentInstance;
        spyOn(component, 'getPermission').and.returnValue(undefined);
        fixture.detectChanges();
    });
    it('should creat', () => {
        expect(component).toBeTruthy();
    });
    it('should get scanner and render', () => {
        component.hasCreatePermission = true;
        let el: HTMLElement =
            fixture.nativeElement.querySelector('#scanner-name');
        expect(el.textContent.trim()).toEqual('test1');
    });
    it('select another scanner', () => {
        component.hasCreatePermission = true;
        component.getScanners();
        fixture.detectChanges();
        const editButton = fixture.nativeElement.querySelector('#edit-scanner');
        expect(editButton).toBeTruthy();
        editButton.click();
        fixture.detectChanges();
        component.selectedScanner = mockScanner2;
        fixture.detectChanges();
        const saveButton = fixture.nativeElement.querySelector('#save-scanner');
        saveButton.click();
        fixture.detectChanges();
        expect(component.opened).toBeFalsy();
    });
    describe('role-based redirect', () => {
        let navigate: jasmine.Spy;
        let init: jasmine.Spy;
        const setRoleIds = (ids: number[]) =>
            ((fakedRoute.snapshot.parent.parent as any).data = {
                projectResolver: { current_user_role_ids: ids },
            });
        beforeEach(() => {
            navigate = spyOn(TestBed.inject(Router), 'navigate');
            init = spyOn(component, 'init');
            (component.getPermission as jasmine.Spy).calls.reset();
        });
        afterEach(() => delete (fakedRoute.snapshot.parent.parent as any).data);

        it('redirects guest-only members and skips loading', () => {
            setRoleIds([3, 5]);
            component.ngOnInit();
            expect(navigate).toHaveBeenCalledWith([
                CommonRoutes.HARBOR_DEFAULT,
            ]);
            expect(component.getPermission).not.toHaveBeenCalled();
            expect(init).not.toHaveBeenCalled();
        });

        it('keeps members with a custom or stronger role', () => {
            setRoleIds([3, 42]);
            component.ngOnInit();
            expect(navigate).not.toHaveBeenCalled();
            expect(component.getPermission).toHaveBeenCalled();
            expect(init).toHaveBeenCalled();
        });
    });
});
