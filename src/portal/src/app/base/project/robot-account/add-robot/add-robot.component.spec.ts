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
import { AddRobotComponent } from './add-robot.component';
import { of } from 'rxjs';
import { MessageHandlerService } from '../../../../shared/services/message-handler.service';
import { delay } from 'rxjs/operators';
import { RobotService } from '../../../../../../ng-swagger-gen/services/robot.service';
import { OperationService } from '../../../../shared/components/operation/operation.service';
import { NO_ERRORS_SCHEMA } from '@angular/core';
import { SharedTestingModule } from '../../../../shared/shared.module';

describe('AddRobotComponent', () => {
    let component: AddRobotComponent;
    let fixture: ComponentFixture<AddRobotComponent>;
    const fakedRobotService = {
        ListRobot() {
            return of([]).pipe(delay(0));
        },
        CreateRobot() {
            return of({});
        },
    };
    const fakedMessageHandlerService = {
        showSuccess() {},
        error() {},
    };
    beforeEach(async () => {
        await TestBed.configureTestingModule({
            declarations: [AddRobotComponent],
            imports: [SharedTestingModule],
            providers: [
                OperationService,
                { provide: RobotService, useValue: fakedRobotService },
                {
                    provide: MessageHandlerService,
                    useValue: fakedMessageHandlerService,
                },
            ],
            schemas: [NO_ERRORS_SCHEMA],
        }).compileComponents();
    });

    beforeEach(() => {
        fixture = TestBed.createComponent(AddRobotComponent);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    describe('provided secret', () => {
        it('should flag a weak secret as invalid with errors', () => {
            component.userProvidedSecret = 'weak';
            component.validateSecret();
            expect(component.secretValidationErrors.length).toBeGreaterThan(0);
            expect(component.isSecretInputValid()).toBeFalsy();
        });

        it('should accept a secret meeting all requirements', () => {
            component.userProvidedSecret = 'StrongPass123';
            component.validateSecret();
            expect(component.secretValidationErrors.length).toBe(0);
            expect(component.isSecretInputValid()).toBeTruthy();
        });

        it('should detect mismatched confirmation', () => {
            component.userProvidedSecret = 'StrongPass123';
            component.userProvidedSecretConfirm = 'Different123';
            expect(component.secretsMatch()).toBeFalsy();
        });

        it('should confirm matching secrets', () => {
            component.userProvidedSecret = 'StrongPass123';
            component.userProvidedSecretConfirm = 'StrongPass123';
            expect(component.secretsMatch()).toBeTruthy();
        });

        it('should not gate creation when no secret is provided', () => {
            component.userProvidedSecret = '';
            expect(component.isProvidedSecretAcceptable()).toBeTruthy();
        });

        it('should block creation when the provided secret is invalid', () => {
            component.userProvidedSecret = 'weak';
            component.userProvidedSecretConfirm = 'weak';
            expect(component.isProvidedSecretAcceptable()).toBeFalsy();
        });

        it('should block creation when the confirmation does not match', () => {
            component.userProvidedSecret = 'StrongPass123';
            component.userProvidedSecretConfirm = 'Mismatch123';
            expect(component.isProvidedSecretAcceptable()).toBeFalsy();
        });

        it('should allow creation when a valid secret is confirmed', () => {
            component.userProvidedSecret = 'StrongPass123';
            component.userProvidedSecretConfirm = 'StrongPass123';
            expect(component.isProvidedSecretAcceptable()).toBeTruthy();
        });

        it('should toggle secret visibility', () => {
            expect(component.showSecretPassword).toBeFalsy();
            component.toggleSecretVisibility();
            expect(component.showSecretPassword).toBeTruthy();
        });

        it('should send the provided secret in the create request', () => {
            const robotService = TestBed.inject(RobotService);
            const createSpy = spyOn(
                robotService,
                'CreateRobot'
            ).and.callThrough();
            component.projectId = 1;
            component.projectName = 'library';
            component.robot.name = 'testrobot';
            component.robot.permissions[0].access = [
                { resource: 'repository', action: 'pull' },
            ];
            component.userProvidedSecret = 'StrongPass123';
            component.userProvidedSecretConfirm = 'StrongPass123';
            component.save();
            expect(createSpy).toHaveBeenCalled();
            const callArgs = createSpy.calls.mostRecent().args[0];
            expect(callArgs.robot.secret).toBe('StrongPass123');
        });

        it('should not send a secret when none was provided', () => {
            const robotService = TestBed.inject(RobotService);
            const createSpy = spyOn(
                robotService,
                'CreateRobot'
            ).and.callThrough();
            component.projectId = 1;
            component.projectName = 'library';
            component.robot.name = 'testrobot';
            component.robot.permissions[0].access = [
                { resource: 'repository', action: 'pull' },
            ];
            component.save();
            expect(createSpy).toHaveBeenCalled();
            const callArgs = createSpy.calls.mostRecent().args[0];
            expect(callArgs.robot.secret).toBeFalsy();
        });

        it('should reset secret fields on reset()', () => {
            component.userProvidedSecret = 'StrongPass123';
            component.userProvidedSecretConfirm = 'StrongPass123';
            component.isSecretDirty = true;
            component.reset();
            expect(component.userProvidedSecret).toBe('');
            expect(component.userProvidedSecretConfirm).toBe('');
            expect(component.isSecretDirty).toBeFalsy();
        });
    });
});
