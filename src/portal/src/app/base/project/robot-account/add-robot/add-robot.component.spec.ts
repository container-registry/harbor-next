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
        it('should default to accepting no secret', () => {
            expect(component.userProvidedSecret).toBe('');
            expect(component.isProvidedSecretAcceptable).toBeTruthy();
        });

        it('should block creation when the shared secret input reports invalid', () => {
            component.robot.name = 'testrobot';
            component.robot.permissions[0].access = [
                { resource: 'repository', action: 'pull' },
            ];
            component.isProvidedSecretAcceptable = false;
            expect(component.canAdd()).toBeFalsy();
        });

        it('should allow creation when the shared secret input reports valid', () => {
            component.robot.name = 'testrobot';
            component.robot.permissions[0].access = [
                { resource: 'repository', action: 'pull' },
            ];
            component.isProvidedSecretAcceptable = true;
            expect(component.canAdd()).toBeTruthy();
        });

        it('should ignore secret acceptability while editing', () => {
            component.isEditMode = true;
            component.robot.name = 'testrobot';
            component.robot.permissions[0].access = [
                { resource: 'repository', action: 'pull' },
            ];
            component.originalRobotForEdit = component.robot;
            component.isProvidedSecretAcceptable = false;
            expect(component.canAdd()).toBeTruthy();
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

        it('should reset secret state on reset()', () => {
            component.userProvidedSecret = 'StrongPass123';
            component.isProvidedSecretAcceptable = false;
            component.reset();
            expect(component.userProvidedSecret).toBe('');
            expect(component.isProvidedSecretAcceptable).toBeTruthy();
        });
    });
});
