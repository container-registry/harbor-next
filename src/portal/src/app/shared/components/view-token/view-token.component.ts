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
import { Component, EventEmitter, Output, ViewChild } from '@angular/core';
import { RobotService } from '../../../../../ng-swagger-gen/services/robot.service';
import { ClrLoadingState } from '@clr/angular';
import { Robot } from '../../../../../ng-swagger-gen/models/robot';
import { clone } from '../../units/utils';
import { NgForm } from '@angular/forms';
import {
    operateChanges,
    OperateInfo,
    OperationState,
} from '../operation/operate';
import { OperationService } from '../operation/operation.service';
import { MessageHandlerService } from '../../services/message-handler.service';
import { DomSanitizer, SafeUrl } from '@angular/platform-browser';
import { TranslateService } from '@ngx-translate/core';
import { InlineAlertComponent } from '../inline-alert/inline-alert.component';
import { errorHandler } from '../../units/shared.utils';
import { CopyInputComponent } from '../push-image/copy-input.component';
import { FederatedIdpService } from 'ng-swagger-gen/services';

@Component({
    selector: 'view-token',
    templateUrl: './view-token.component.html',
    styleUrls: ['./view-token.component.scss'],
})
export class ViewTokenComponent {
    showNewPwd: boolean = false;
    showConfirmPwd: boolean = false;
    tokenModalOpened: boolean = false;
    private _robot: Robot;
    inheritedClaims: { path: string; value: string }[];
    claims: { path: string; value: string }[];

    // Use getter/setter to trigger claims fetch when robot is set
    get robot(): Robot {
        return this._robot;
    }

    set robot(value: Robot) {
        this._robot = value;
        // Fetch claims when robot is set and has federatedidp_id
        if (value?.federatedidp_id > 0) {
            this.fetchClaimsForRobot(value.federatedidp_id, value.id);
        } else {
            // Clear claims if not a federated robot
            this.inheritedClaims = [];
            this.claims = [];
        }
    }

    // Keep direct access to the robot property for backward compatibility
    setRobotDirect(value: Robot) {
        this._robot = value;
    }
    newSecret: string;
    confirmSecret: string;
    btnState: ClrLoadingState = ClrLoadingState.DEFAULT;
    @ViewChild(InlineAlertComponent)
    inlineAlertComponent: InlineAlertComponent;
    @ViewChild('secretForm', { static: true }) secretForm: NgForm;
    @ViewChild('copyInputComponent')
    copyInputComponent: CopyInputComponent;
    @Output()
    refreshSuccess: EventEmitter<boolean> = new EventEmitter<boolean>();
    copyToken: boolean = false;
    createSuccess: string;
    downLoadFileName: string = '';
    downLoadHref: SafeUrl = '';
    enableNewSecret: boolean = false;

    constructor(
        private robotService: RobotService,
        private idpService: FederatedIdpService,
        private operationService: OperationService,
        private msgHandler: MessageHandlerService,
        private sanitizer: DomSanitizer,
        private translate: TranslateService
    ) {}

    cancel() {
        this.tokenModalOpened = false;
    }
    open() {
        this.showNewPwd = false;
        this.showConfirmPwd = false;
        this.tokenModalOpened = true;
        this.inlineAlertComponent.close();
        this.copyToken = false;
        this.createSuccess = null;
        this.newSecret = null;
        this.confirmSecret = null;
        this.downLoadFileName = '';
        this.downLoadHref = '';
        this.secretForm.reset();
        // Note: Claims fetch is now handled by the robot setter
    }
    refreshToken() {
        this.btnState = ClrLoadingState.LOADING;
        const robot: Robot = clone(this.robot);
        const opeMessage = new OperateInfo();
        opeMessage.name = 'SYSTEM_ROBOT.REFRESH_SECRET';
        opeMessage.data.id = robot.id;
        opeMessage.state = OperationState.progressing;
        opeMessage.data.name = robot.name;
        this.operationService.publishInfo(opeMessage);
        if (this.newSecret) {
            robot.secret = this.newSecret;
        }
        this.robotService
            .RefreshSec({
                robotId: robot.id,
                robotSec: {
                    secret: robot.secret,
                },
            })
            .subscribe(
                res => {
                    this.btnState = ClrLoadingState.SUCCESS;
                    operateChanges(opeMessage, OperationState.success);
                    this.refreshSuccess.emit(true);
                    this.cancel();
                    if (res && res.secret) {
                        // Update secret directly without triggering the robot setter
                        this._robot.secret = res.secret;
                        this.copyToken = true;
                        this.createSuccess =
                            'SYSTEM_ROBOT.REFRESH_SECRET_SUCCESS';
                        // export to token file
                        robot.secret = res.secret;
                        const downLoadUrl = `data:text/json;charset=utf-8, ${encodeURIComponent(
                            JSON.stringify(robot)
                        )}`;
                        this.downLoadHref =
                            this.sanitizer.bypassSecurityTrustUrl(downLoadUrl);
                        this.downLoadFileName = `${robot.name}.json`;
                    } else {
                        this.msgHandler.showSuccess(
                            'SYSTEM_ROBOT.REFRESH_SECRET_SUCCESS'
                        );
                    }
                },
                error => {
                    this.btnState = ClrLoadingState.ERROR;
                    this.inlineAlertComponent.showInlineError(error);
                    operateChanges(
                        opeMessage,
                        OperationState.failure,
                        errorHandler(error)
                    );
                }
            );
    }
    canRefresh() {
        if (this.enableNewSecret && !this.newSecret && !this.confirmSecret) {
            return false;
        }
        if (!this.newSecret && !this.confirmSecret) {
            return true;
        }
        return (
            this.newSecret &&
            this.confirmSecret &&
            this.newSecret === this.confirmSecret &&
            this.secretForm.valid
        );
    }
    onCpSuccess($event: any): void {
        this.copyToken = false;
        this.tokenModalOpened = false;
        if (this.copyInputComponent) {
            this.copyInputComponent.reset();
        }
        this.translate
            .get('ROBOT_ACCOUNT.COPY_SUCCESS', { param: this.robot.name })
            .subscribe((res: string) => {
                this.msgHandler.showSuccess(res);
            });
    }
    /**
     * Fetch claims for a robot - takes robotId as parameter to avoid timing issues
     * with async callbacks where this.robot might not be set yet
     */
    fetchClaimsForRobot(idpID: number, robotId: number) {
        if (robotId === undefined || robotId === null || isNaN(robotId)) {
            return;
        }
        this.idpService.ListClaimRules({ id: idpID }).subscribe(
            claimRules => {
                // Inherited claims are IdP-scoped (robot_id is 0, null, or undefined).
                this.inheritedClaims = claimRules
                    .filter(c => !c.robot_id || c.robot_id === 0)
                    .map(c => ({ path: c.claim_path, value: c.value }));
                // Robot-scoped claims: coerce to number to tolerate string/number drift.
                this.claims = claimRules
                    .filter(c => c.robot_id && +c.robot_id === +robotId)
                    .map(c => ({ path: c.claim_path, value: c.value }));
            },
            error => {
                this.inlineAlertComponent.showInlineError(error);
            }
        );
    }

    closeModal() {
        this.copyToken = false;
        this.tokenModalOpened = false;
    }

    notSame(): boolean {
        return (
            this.secretForm.valid &&
            this.newSecret &&
            this.confirmSecret &&
            this.newSecret !== this.confirmSecret
        );
    }
    changeEnableNewSecret() {
        this.secretForm.reset({
            enableNewSecret: this.enableNewSecret,
        });
    }
}
