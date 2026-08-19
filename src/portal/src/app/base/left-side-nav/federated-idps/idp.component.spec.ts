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
import { IdpComponent } from './idp.component';
import { CreateEditIdpComponent } from './create-edit-idp/create-edit-idp.component';
import { FederatedIdpService } from 'ng-swagger-gen/services';
import { FederatedIdp } from 'ng-swagger-gen/models';
import { SharedTestingModule } from '../../../shared/shared.module';
import { ErrorHandler } from '../../../shared/units/error-handler';
import { OperationService } from '../../../shared/components/operation/operation.service';

describe('IdpComponent', () => {
    const mockIdps: FederatedIdp[] = [
        { id: 1, name: 'idp-one', issuer: 'https://issuer.example.com' },
    ];
    const mockIdpService = {
        ListFederatedIdps: () => of(mockIdps),
        DeleteFederatedIdp: () => of(null),
    };

    let fixture: ComponentFixture<IdpComponent>;
    let component: IdpComponent;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [SharedTestingModule],
            declarations: [IdpComponent, CreateEditIdpComponent],
            providers: [
                ErrorHandler,
                OperationService,
                { provide: FederatedIdpService, useValue: mockIdpService },
            ],
            schemas: [NO_ERRORS_SCHEMA],
        }).compileComponents();

        fixture = TestBed.createComponent(IdpComponent);
        component = fixture.componentInstance;
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should load IdPs from the service on refresh', () => {
        component.refreshTargets();
        expect(component.targets.length).toBe(1);
        expect(component.targets[0].name).toBe('idp-one');
    });
});
