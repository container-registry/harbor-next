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
import { NewRobotComponent } from './new-robot.component';
import { NO_ERRORS_SCHEMA } from '@angular/core';
import { BrowserAnimationsModule } from '@angular/platform-browser/animations';
import { ClarityModule } from '@clr/angular';
import { TranslateModule } from '@ngx-translate/core';
import { Robot } from '../../../../../../ng-swagger-gen/models/robot';
import { Action, PermissionsKinds, Resource } from '../system-robot-util';
import { MessageHandlerService } from '../../../../shared/services/message-handler.service';
import { OperationService } from '../../../../shared/components/operation/operation.service';
import { RobotService } from '../../../../../../ng-swagger-gen/services/robot.service';
import { of } from 'rxjs';
import { delay } from 'rxjs/operators';
import { ConfigurationService } from '../../../../services/config.service';
import { Configuration } from '../../config/config';
import { FormsModule } from '@angular/forms';

describe('NewRobotComponent', () => {
    let component: NewRobotComponent;
    let fixture: ComponentFixture<NewRobotComponent>;
    const robot1: Robot = {
        id: 1,
        name: 'robot1',
        level: PermissionsKinds.SYSTEM,
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
    const fakedRobotService = {
        ListRobot() {
            return of([]).pipe(delay(0));
        },
        CreateRobot() {
            return of({});
        },
    };
    const mockConfigurationService = {
        getConfiguration() {
            const config: Configuration = new Configuration();
            config.robot_token_duration = {
                value: 10000,
                editable: true,
            };
            return of(config).pipe(delay(0));
        },
    };
    const fakedMessageHandlerService = {
        showSuccess() {},
        error() {},
    };
    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                BrowserAnimationsModule,
                ClarityModule,
                TranslateModule.forRoot(),
                FormsModule,
            ],
            declarations: [NewRobotComponent],
            providers: [
                OperationService,
                {
                    provide: MessageHandlerService,
                    useValue: fakedMessageHandlerService,
                },
                { provide: RobotService, useValue: fakedRobotService },
                {
                    provide: ConfigurationService,
                    useValue: mockConfigurationService,
                },
            ],
            schemas: [NO_ERRORS_SCHEMA],
        }).compileComponents();
    });

    beforeEach(() => {
        fixture = TestBed.createComponent(NewRobotComponent);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should show "name is required"', async () => {
        fixture.autoDetectChanges();
        component.isEditMode = false;
        component.addRobotOpened = true;
        await fixture.whenStable();
        const nameInput = fixture.nativeElement.querySelector('#name');
        nameInput.value = '';
        nameInput.dispatchEvent(new Event('input'));
        nameInput.blur();
        nameInput.dispatchEvent(new Event('blur'));
        let el = fixture.nativeElement.querySelector('clr-control-error');
        expect(el).toBeTruthy();
    });
    it('should be edit model', async () => {
        fixture.autoDetectChanges();
        component.isEditMode = true;
        component.addRobotOpened = true;
        component.systemRobot = robot1;
        await fixture.whenStable();
        const nameInput = fixture.nativeElement.querySelector('#name');
        expect(nameInput.value).toEqual('robot1');
    });

    describe('provided secret', () => {
        beforeEach(() => {
            // this test module doesn't declare InlineAlertComponent, so the
            // @ViewChild never resolves; stub it since open()/reset()/cancel()
            // all call inlineAlertComponent.close()
            component.inlineAlertComponent = { close: () => {} } as any;
        });

        it('should default to accepting no secret', () => {
            expect(component.userProvidedSecret).toBe('');
            expect(component.isProvidedSecretAcceptable).toBeTruthy();
        });

        it('should block creation when the shared secret input reports invalid', () => {
            component.systemRobot.name = 'testsystemrobot';
            component.coverAll = true;
            component.permissionForCoverAll.access = [
                { resource: Resource.ARTIFACT, action: Action.PULL },
            ];
            component.isProvidedSecretAcceptable = false;
            fixture.detectChanges();
            expect(component.canAdd()).toBeFalsy();
        });

        it('should allow creation when the shared secret input reports valid', () => {
            component.systemRobot.name = 'testsystemrobot';
            component.coverAll = true;
            component.permissionForCoverAll.access = [
                { resource: Resource.ARTIFACT, action: Action.PULL },
            ];
            component.isProvidedSecretAcceptable = true;
            fixture.detectChanges();
            expect(component.canAdd()).toBeTruthy();
        });

        it('should send the provided secret in the create request', () => {
            const robotService = TestBed.inject(RobotService);
            const createSpy = spyOn(
                robotService,
                'CreateRobot'
            ).and.callThrough();
            component.isEditMode = false;
            component.systemRobot.name = 'testsystemrobot';
            component.coverAll = true;
            component.permissionForCoverAll.access = [
                { resource: Resource.ARTIFACT, action: Action.PULL },
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
            component.isEditMode = false;
            component.systemRobot.name = 'testsystemrobot';
            component.coverAll = true;
            component.permissionForCoverAll.access = [
                { resource: Resource.ARTIFACT, action: Action.PULL },
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
