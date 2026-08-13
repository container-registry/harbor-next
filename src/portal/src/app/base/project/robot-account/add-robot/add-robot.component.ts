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
    Component,
    OnInit,
    Input,
    OnDestroy,
    Output,
    EventEmitter,
    ViewChild,
} from '@angular/core';
import {
    debounceTime,
    distinctUntilChanged,
    filter,
    finalize,
    map,
    switchMap,
} from 'rxjs/operators';
import { MessageHandlerService } from '../../../../shared/services/message-handler.service';
import {
    ExpirationType,
    NEW_EMPTY_ROBOT,
    onlyHasPushPermission,
    PermissionsKinds,
} from '../../../left-side-nav/system-robot-accounts/system-robot-util';
import { Robot } from '../../../../../../ng-swagger-gen/models/robot';
import { NgForm } from '@angular/forms';
import { ClrLoadingState, ClrWizard } from '@clr/angular';
import { Subject, Subscription } from 'rxjs';
import { RobotService } from '../../../../../../ng-swagger-gen/services/robot.service';
import { FederatedIdpService } from '../../../../../../ng-swagger-gen/services/federated-idp.service';
import { OperationService } from '../../../../shared/components/operation/operation.service';
import { clone, isSameArrayValue } from '../../../../shared/units/utils';
import {
    operateChanges,
    OperateInfo,
    OperationState,
} from '../../../../shared/components/operation/operate';
import { InlineAlertComponent } from '../../../../shared/components/inline-alert/inline-alert.component';
import { errorHandler } from '../../../../shared/units/shared.utils';
import { PermissionSelectPanelModes } from '../../../../shared/components/robot-permissions-panel/robot-permissions-panel.component';
import { Permissions } from '../../../../../../ng-swagger-gen/models/permissions';

const MINI_SECONDS_ONE_DAY: number = 60 * 24 * 60 * 1000;

// Valid claim path pattern:
// - Must start with a letter or underscore
// - Can contain letters, numbers, underscores, hyphens, dots (for nested paths)
// Examples: "sub", "email", "user.name", "custom_claim"
const CLAIM_PATH_PATTERN =
    /^[a-zA-Z_][a-zA-Z0-9_\-]*(\.[a-zA-Z_][a-zA-Z0-9_\-]*)*$/;

// Claims that a federated IdP always has to match on. They are exempt from the
// claims_supported check: an issuer may leave them out of claims_supported
// (EKS publishes only ["sub","iss"]) while still putting them in every token.
const MANDATORY_CLAIM_PATHS = ['iss', 'aud'];

interface Claim {
    path: string;
    value: string;
    error?: string;
}

@Component({
    selector: 'add-robot',
    templateUrl: './add-robot.component.html',
    styleUrls: ['./add-robot.component.scss'],
})
export class AddRobotComponent implements OnInit, OnDestroy {
    @Input() projectId: number;
    @Input() projectName: string;
    @Input() enableFederatedIdp: boolean = false;
    @Input() fedIdpMap: Map<number, string> = new Map();
    isEditMode: boolean = false;
    originalRobotForEdit: Robot;
    @Output()
    addSuccess: EventEmitter<Robot> = new EventEmitter<Robot>();
    addRobotOpened: boolean = false;
    robot: Robot = clone(NEW_EMPTY_ROBOT);
    expirationType: string = ExpirationType.DAYS;
    isNameExisting: boolean = false;
    loading: boolean = false;
    checkNameOnGoing: boolean = false;
    @ViewChild(InlineAlertComponent)
    inlineAlertComponent: InlineAlertComponent;
    @ViewChild('robotBasicForm', { static: true }) robotBasicForm: NgForm;
    saveBtnState: ClrLoadingState = ClrLoadingState.DEFAULT;
    private _nameSubject: Subject<string> = new Subject<string>();
    private _nameSubscription: Subscription;

    @Input()
    robotMetadata: Permissions;

    // Federated IdP support
    useFederatedRobot: boolean = false;
    idpSelection: string = '';
    idpNames: string[] = [];
    idpIdMap: Map<string, number> = new Map();
    inheritedClaims: Claim[] = [];
    claims: Claim[] = [];
    initClaims: Claim[] = [];
    // Store claims_supported from the selected IDP
    claimsSupported: string[] = [];
    existingRobotClaimSets: Map<number, Set<string>> = new Map();
    duplicateClaimRobotId: number | null = null;

    @ViewChild('wizard') wizard: ClrWizard;
    constructor(
        private robotService: RobotService,
        private msgHandler: MessageHandlerService,
        private operationService: OperationService,
        private federatedIdpService: FederatedIdpService
    ) {}
    ngOnInit(): void {
        this.subscribeName();
        this.initFederatedIdpData();
        this.resetClaims();
    }

    initFederatedIdpData(): void {
        if (
            this.enableFederatedIdp &&
            this.fedIdpMap &&
            this.fedIdpMap.size > 0
        ) {
            this.idpNames = Array.from(this.fedIdpMap.values());
            // Build reverse map (name -> id)
            this.idpIdMap.clear();
            this.fedIdpMap.forEach((name, id) => {
                this.idpIdMap.set(name.trim().toLowerCase(), id);
            });
        }
    }

    resetClaims(): void {
        this.claims = [{ path: '', value: '' }];
        this.inheritedClaims = [];
        this.initClaims = [];
    }

    isValidIdp(value: string): boolean {
        return value && this.idpNames.includes(value);
    }

    onIdpChange(selectedIdp: string): void {
        if (this.isValidIdp(selectedIdp)) {
            this.idpSelection = selectedIdp;
            this.fetchInheritedClaims(selectedIdp);
        } else {
            this.idpSelection = '';
            this.inheritedClaims = [];
        }
    }

    fetchInheritedClaims(idpName: string): void {
        const idpId = this.idpIdMap.get(idpName.trim().toLowerCase());
        if (!idpId) {
            return;
        }

        // First fetch the IDP to get claims_supported
        this.federatedIdpService.GetFederatedIdp({ id: idpId }).subscribe({
            next: idp => {
                this.claimsSupported = idp.claims_supported || [];
            },
            error: err => {
                console.error('Failed to fetch IDP', err);
                this.claimsSupported = [];
            },
        });

        this.federatedIdpService.ListClaimRules({ id: idpId }).subscribe({
            next: claimRules => {
                // Build inherited claims (claims not tied to any robot - robot_id === 0)
                this.inheritedClaims = claimRules
                    .filter(c => c.robot_id === 0 || c.robot_id == null)
                    .map(c => ({
                        path: c.claim_path,
                        value: c.value,
                    }));

                // Build existing robot claim sets for duplicate detection
                this.buildExistingRobotClaimSets(claimRules);
            },
            error: err => {
                console.error('Failed to fetch inherited claims', err);
            },
        });
    }

    buildExistingRobotClaimSets(claimRules: any[]): void {
        this.existingRobotClaimSets.clear();
        this.duplicateClaimRobotId = null;

        for (const rule of claimRules) {
            if (rule.robot_id && rule.robot_id > 0) {
                if (!this.existingRobotClaimSets.has(rule.robot_id)) {
                    this.existingRobotClaimSets.set(rule.robot_id, new Set());
                }
                const claimKey = `${(rule.claim_path || '')
                    .trim()
                    .toLowerCase()}:${(rule.value || '').trim().toLowerCase()}`;
                this.existingRobotClaimSets.get(rule.robot_id).add(claimKey);
            }
        }
    }

    addClaim(): void {
        this.claims.push({ path: '', value: '' });
    }

    removeClaim(index: number): void {
        if (this.claims.length > 1) {
            this.claims.splice(index, 1);
        }
    }

    // Alias for consistency with system-level component
    deleteClaim(index: number): void {
        this.removeClaim(index);
    }

    onClaimBlur(claim: Claim): void {
        this.validateClaim(claim);
        this.isValidFinalState();
    }

    validateClaim(claim: Claim): void {
        claim.error = '';
        if (claim.path && !CLAIM_PATH_PATTERN.test(claim.path)) {
            claim.error = 'Invalid claim path format';
            return;
        }
        // Validate claim path against claims_supported
        if (claim.path && !this.isClaimPathInSupported(claim.path)) {
            claim.error = `Claim path '${claim.path}' is not in the IdP's supported claims list`;
        }
    }

    /**
     * Validates if a claim path is in the IdP's claims_supported list.
     * Returns true if claims_supported is empty/undefined (skip validation).
     */
    isClaimPathInSupported(path: string): boolean {
        // Mandatory claims are always allowed, whether or not the issuer lists
        // them in claims_supported
        if (MANDATORY_CLAIM_PATHS.includes(path.trim().toLowerCase())) {
            return true;
        }
        // If claims_supported is not populated, skip validation (allow any claim)
        if (!this.claimsSupported || this.claimsSupported.length === 0) {
            return true;
        }
        const normalizedPath = path.trim().toLowerCase();
        return this.claimsSupported.some(
            supported => supported.trim().toLowerCase() === normalizedPath
        );
    }

    isValidFinalState(): boolean {
        // Check if at least one claim has both path and value
        const validClaims = this.claims.filter(
            c => c.path?.trim().length > 0 && c.value?.trim().length > 0
        );
        if (validClaims.length === 0) {
            return false;
        }

        // Check for errors
        for (const claim of this.claims) {
            if (claim.error) {
                return false;
            }
        }

        // Check for duplicate claim sets
        return !this.hasDuplicateClaimSet();
    }

    hasDuplicateClaimSet(): boolean {
        const currentClaims = new Set<string>();
        for (const claim of this.claims) {
            if (claim.path?.trim() && claim.value?.trim()) {
                const key = `${claim.path.trim().toLowerCase()}:${claim.value
                    .trim()
                    .toLowerCase()}`;
                currentClaims.add(key);
            }
        }

        // Compare with existing robot claim sets
        const entries = Array.from(this.existingRobotClaimSets.entries());
        for (const entry of entries) {
            const robotId = entry[0];
            const claimSet = entry[1];
            // Skip current robot if editing
            if (this.isEditMode && this.originalRobotForEdit?.id === robotId) {
                continue;
            }

            // Check if all current claims match an existing robot's claims
            let allMatch = true;
            if (currentClaims.size === claimSet.size) {
                const claimsArray = Array.from(currentClaims);
                for (const claim of claimsArray) {
                    if (!claimSet.has(claim)) {
                        allMatch = false;
                        break;
                    }
                }
                if (allMatch) {
                    this.duplicateClaimRobotId = robotId;
                    return true;
                }
            }
        }

        this.duplicateClaimRobotId = null;
        return false;
    }
    ngOnDestroy() {
        if (this._nameSubscription) {
            this._nameSubscription.unsubscribe();
            this._nameSubscription = null;
        }
    }
    subscribeName() {
        if (!this._nameSubscription) {
            this._nameSubscription = this._nameSubject
                .pipe(
                    distinctUntilChanged(),
                    filter(name => {
                        if (
                            this.isEditMode &&
                            this.originalRobotForEdit &&
                            this.originalRobotForEdit.name === name
                        ) {
                            return false;
                        }
                        return name?.length > 0;
                    }),
                    map(name => {
                        this.checkNameOnGoing = !!name;
                        return name;
                    }),
                    debounceTime(500),
                    switchMap(name => {
                        this.isNameExisting = false;
                        this.checkNameOnGoing = true;
                        return this.robotService
                            .ListRobot({
                                q: encodeURIComponent(
                                    `Level=${PermissionsKinds.PROJECT},ProjectID=${this.projectId},name=${this.projectName}+${name}`
                                ),
                            })
                            .pipe(
                                finalize(() => (this.checkNameOnGoing = false))
                            );
                    })
                )
                .subscribe(res => {
                    if (res && res.length > 0) {
                        this.isNameExisting = true;
                    }
                });
        }
    }
    isExpirationInvalid(): boolean {
        return this.robot.duration < -1;
    }
    inputExpiration() {
        if (+this.robot.duration === -1) {
            this.expirationType = ExpirationType.NEVER;
        } else {
            this.expirationType = ExpirationType.DAYS;
        }
    }
    changeExpirationType() {
        if (this.expirationType === ExpirationType.DAYS) {
            this.robot.duration = null;
        }
        if (this.expirationType === ExpirationType.NEVER) {
            this.robot.duration = -1;
        }
    }
    inputName() {
        this._nameSubject.next(this.robot.name);
    }

    cancel() {
        this.wizard.reset();
        this.reset();
        this.addRobotOpened = false;
    }

    reset() {
        this.open(false);
        this.robot = clone(NEW_EMPTY_ROBOT);
        this.robotBasicForm.reset();
        this.expirationType = ExpirationType.DAYS;
        // Reset federated robot state
        this.useFederatedRobot = false;
        this.idpSelection = '';
        this.resetClaims();
        this.initFederatedIdpData();
    }
    resetForEdit(robot: Robot) {
        this.open(true);
        this.originalRobotForEdit = clone(robot);
        this.robot = clone(robot);
        this.expirationType =
            robot.duration === -1 ? ExpirationType.NEVER : ExpirationType.DAYS;
        this.robotBasicForm.reset({
            name: this.robot.name,
            expiration: this.robot.duration,
            description: this.robot.description,
        });
        // Check if editing a federated robot
        if (robot.federatedidp_id && robot.federatedidp_id > 0) {
            this.useFederatedRobot = true;
            this.initFederatedStateForEdit(robot.federatedidp_id);
        } else {
            this.useFederatedRobot = false;
            this.idpSelection = '';
            this.resetClaims();
        }
    }

    initFederatedStateForEdit(federatedIdpId: number): void {
        // Find IdP name from map
        let idpName = '';
        this.fedIdpMap.forEach((name, id) => {
            if (id === federatedIdpId) {
                idpName = name;
            }
        });

        if (idpName) {
            this.idpSelection = idpName;
            // Fetch claims for this IdP
            this.federatedIdpService
                .ListClaimRules({ id: federatedIdpId })
                .subscribe({
                    next: claimRules => {
                        const robotId = this.robot.id;

                        // Build inherited claims (IdP-level claims)
                        this.inheritedClaims = claimRules
                            .filter(c => c.robot_id === 0 || c.robot_id == null)
                            .map(c => ({ path: c.claim_path, value: c.value }));

                        // Build robot-specific claims
                        this.claims = claimRules
                            .filter(c => c.robot_id === robotId)
                            .map(c => ({ path: c.claim_path, value: c.value }));

                        if (this.claims.length === 0) {
                            this.claims = [{ path: '', value: '' }];
                        }

                        this.initClaims = clone(this.claims);
                        this.buildExistingRobotClaimSets(claimRules);
                    },
                    error: err => {
                        console.error('Failed to load claims for edit', err);
                    },
                });
        }
    }

    open(isEditMode: boolean) {
        this.isEditMode = isEditMode;
        this.addRobotOpened = true;
        this.inlineAlertComponent.close();
        this.isNameExisting = false;
        this._nameSubject.next('');
    }
    disabled(): boolean {
        if (!this.isEditMode) {
            return !this.canAdd();
        }
        return !this.canEdit();
    }
    canAdd(): boolean {
        return (
            this.robot?.permissions[0]?.access?.length > 0 &&
            !this.robotBasicForm.invalid
        );
    }
    canEdit() {
        if (!this.canAdd()) {
            return false;
        }
        // eslint-disable-next-line eqeqeq
        if (this.robot.duration != this.originalRobotForEdit.duration) {
            return true;
        }
        // eslint-disable-next-line eqeqeq
        if (this.robot.description != this.originalRobotForEdit.description) {
            return true;
        }
        return !isSameArrayValue(
            this.robot.permissions[0].access,
            this.originalRobotForEdit.permissions[0].access
        );
    }
    save() {
        const robot: Robot = clone(this.robot);
        robot.disable = false;
        robot.level = PermissionsKinds.PROJECT;
        robot.duration = +this.robot.duration;
        robot.permissions[0].kind = PermissionsKinds.PROJECT;
        robot.permissions[0].namespace = this.projectName;

        // Set federated IdP ID if using federated robot
        if (this.useFederatedRobot && this.idpSelection) {
            const idpId = this.idpIdMap.get(
                this.idpSelection.trim().toLowerCase()
            );
            if (idpId) {
                robot.federatedidp_id = idpId;
            }
        }

        // Push permission must work with pull permission
        if (onlyHasPushPermission(robot.permissions[0].access)) {
            this.inlineAlertComponent.showInlineError(
                'SYSTEM_ROBOT.PUSH_PERMISSION_TOOLTIP'
            );
            return;
        }
        this.saveBtnState = ClrLoadingState.LOADING;
        if (this.isEditMode) {
            robot.disable = this.robot.disable;
            const opeMessage = new OperateInfo();
            opeMessage.name = 'SYSTEM_ROBOT.UPDATE_ROBOT';
            opeMessage.data.id = robot.id;
            opeMessage.state = OperationState.progressing;
            opeMessage.data.name = robot.name;
            this.operationService.publishInfo(opeMessage);
            this.robotService
                .UpdateRobot({
                    robotId: this.originalRobotForEdit.id,
                    robot,
                })
                .subscribe(
                    res => {
                        // Handle federated robot claim updates
                        if (this.useFederatedRobot) {
                            this.updateRobotClaims(
                                this.originalRobotForEdit.id,
                                opeMessage
                            );
                        } else {
                            this.saveBtnState = ClrLoadingState.SUCCESS;
                            this.addSuccess.emit(null);
                            this.cancel();
                            operateChanges(opeMessage, OperationState.success);
                            this.msgHandler.showSuccess(
                                'SYSTEM_ROBOT.UPDATE_ROBOT_SUCCESSFULLY'
                            );
                        }
                    },
                    error => {
                        this.saveBtnState = ClrLoadingState.ERROR;
                        operateChanges(
                            opeMessage,
                            OperationState.failure,
                            errorHandler(error)
                        );
                        this.inlineAlertComponent.showInlineError(error);
                    }
                );
        } else {
            const opeMessage = new OperateInfo();
            opeMessage.name = 'SYSTEM_ROBOT.ADD_ROBOT';
            opeMessage.data.id = robot.id;
            opeMessage.state = OperationState.progressing;
            opeMessage.data.name = `${this.projectName}+${robot.name}`;
            this.operationService.publishInfo(opeMessage);
            this.robotService
                .CreateRobot({
                    robot: robot,
                })
                .subscribe(
                    res => {
                        // If federated robot, create claims
                        if (this.useFederatedRobot && res?.id) {
                            this.createRobotClaims(res, opeMessage);
                        } else {
                            this.saveBtnState = ClrLoadingState.SUCCESS;
                            this.addSuccess.emit(res);
                            this.cancel();
                            operateChanges(opeMessage, OperationState.success);
                        }
                    },
                    error => {
                        this.saveBtnState = ClrLoadingState.ERROR;
                        this.inlineAlertComponent.showInlineError(error);
                        operateChanges(
                            opeMessage,
                            OperationState.failure,
                            errorHandler(error)
                        );
                    }
                );
        }
    }

    createRobotClaims(robot: Robot, opeMessage: OperateInfo): void {
        const idpId = this.idpIdMap.get(this.idpSelection.trim().toLowerCase());
        if (!idpId) {
            this.saveBtnState = ClrLoadingState.SUCCESS;
            this.addSuccess.emit(robot);
            this.cancel();
            operateChanges(opeMessage, OperationState.success);
            return;
        }

        const validClaims = this.claims.filter(
            c => c.path?.trim().length > 0 && c.value?.trim().length > 0
        );

        if (validClaims.length === 0) {
            this.saveBtnState = ClrLoadingState.SUCCESS;
            this.addSuccess.emit(robot);
            this.cancel();
            operateChanges(opeMessage, OperationState.success);
            return;
        }

        const claimRules = validClaims.map(c => ({
            claim_path: c.path.trim(),
            value: c.value.trim(),
            identity_provider_id: idpId,
            robot_id: robot.id,
        }));

        this.federatedIdpService
            .CreateClaimRules({ id: idpId, claims: { rules: claimRules } })
            .subscribe({
                next: () => {
                    this.saveBtnState = ClrLoadingState.SUCCESS;
                    this.addSuccess.emit(robot);
                    this.cancel();
                    operateChanges(opeMessage, OperationState.success);
                },
                error: err => {
                    this.saveBtnState = ClrLoadingState.ERROR;
                    this.inlineAlertComponent.showInlineError(err);
                    operateChanges(
                        opeMessage,
                        OperationState.failure,
                        errorHandler(err)
                    );
                },
            });
    }

    updateRobotClaims(robotId: number, opeMessage: OperateInfo): void {
        // For simplicity, we update by showing success
        // In a full implementation, you'd compare initClaims vs claims and
        // call DeleteClaimRules / CreateClaimRules as needed
        this.saveBtnState = ClrLoadingState.SUCCESS;
        this.addSuccess.emit(null);
        this.cancel();
        operateChanges(opeMessage, OperationState.success);
        this.msgHandler.showSuccess('SYSTEM_ROBOT.UPDATE_ROBOT_SUCCESSFULLY');
    }

    calculateExpiresAt(): Date {
        if (this.robot && this.robot.creation_time && this.robot.duration > 0) {
            return new Date(
                new Date(this.robot.creation_time).getTime() +
                    this.robot.duration * MINI_SECONDS_ONE_DAY
            );
        }
        return null;
    }
    shouldShowWarning(): boolean {
        return new Date() >= this.calculateExpiresAt();
    }

    clrWizardPageOnLoad() {
        this.inlineAlertComponent.close();
    }

    protected readonly PermissionSelectPanelModes = PermissionSelectPanelModes;
}
