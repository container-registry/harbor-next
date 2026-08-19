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
    EventEmitter,
    Input,
    OnDestroy,
    OnInit,
    Output,
    ViewChild,
} from '@angular/core';
import { ConfigurationService } from '../../../../services/config.service';
import { Robot } from '../../../../../../ng-swagger-gen/models/robot';
import { ListAllProjectsComponent } from '../list-all-projects/list-all-projects.component';
import { NgForm } from '@angular/forms';
import {
    debounceTime,
    distinctUntilChanged,
    filter,
    finalize,
    map,
    switchMap,
} from 'rxjs/operators';
import {
    ExpirationType,
    getSystemAccess,
    NAMESPACE_ALL_PROJECTS,
    NAMESPACE_SYSTEM,
    NEW_EMPTY_ROBOT,
    onlyHasPushPermission,
    PermissionsKinds,
} from '../system-robot-util';
import {
    clone,
    isSameArrayValue,
    isSameObject,
} from '../../../../shared/units/utils';
import { RobotService } from '../../../../../../ng-swagger-gen/services/robot.service';
import { ClrLoadingState, ClrWizard } from '@clr/angular';
import { MessageHandlerService } from '../../../../shared/services/message-handler.service';
import { Subject, Subscription } from 'rxjs';
import {
    operateChanges,
    OperateInfo,
    OperationState,
} from '../../../../shared/components/operation/operate';
import { OperationService } from '../../../../shared/components/operation/operation.service';
import { InlineAlertComponent } from '../../../../shared/components/inline-alert/inline-alert.component';
import { errorHandler } from '../../../../shared/units/shared.utils';
import { RobotPermission } from '../../../../../../ng-swagger-gen/models/robot-permission';
import { PermissionSelectPanelModes } from '../../../../shared/components/robot-permissions-panel/robot-permissions-panel.component';
import { Permissions } from '../../../../../../ng-swagger-gen/models/permissions';
import { FederatedIdpService } from 'ng-swagger-gen/services';
import { FederatedIdp } from 'ng-swagger-gen/models';

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
    error?: string; // optional so it doesn't break existing code
}

@Component({
    selector: 'new-robot',
    templateUrl: './new-robot.component.html',
    styleUrls: ['./new-robot.component.scss'],
})
export class NewRobotComponent implements OnInit, OnDestroy {
    isEditMode: boolean = false;
    originalRobotForEdit: Robot;
    @Output()
    addSuccess: EventEmitter<Robot> = new EventEmitter<Robot>();
    addRobotOpened: boolean = false;
    systemRobot: Robot = clone(NEW_EMPTY_ROBOT);
    useFederatedRobot: boolean = false;
    // Array to store claim data
    claims: Claim[] = [];
    initClaims: { path: string; value: string }[];
    inheritedClaims: { path: string; value: string }[];
    // Store claims_supported from the selected IDP
    claimsSupported: string[] = [];
    expirationType: string = ExpirationType.DAYS;
    systemExpirationDays: number;
    coverAll: boolean = false;
    coverAllForEdit: boolean = false;

    isNameExisting: boolean = false;
    loading: boolean = false;
    checkNameOnGoing: boolean = false;
    loadingSystemConfig: boolean = false;
    @ViewChild(ListAllProjectsComponent)
    listAllProjectsComponent: ListAllProjectsComponent;
    @ViewChild(InlineAlertComponent)
    inlineAlertComponent: InlineAlertComponent;
    @ViewChild('robotForm', { static: true }) robotForm: NgForm;
    saveBtnState: ClrLoadingState = ClrLoadingState.DEFAULT;
    private _nameSubject: Subject<string> = new Subject<string>();
    private _nameSubscription: Subscription;
    idpSelection: string;
    idpOptions: any[] = [];
    filteredIdps: any[] = [];
    checkIdpOnGoing = false;
    _idpSubscription: Subscription;
    idpNames: string[] = [];
    idpMap: Map<string, number> = new Map<string, number>();
    loadingIdps: boolean = false;
    // Store claim sets for all robots in the selected IDP, keyed by robot_id
    // Each value is a Set of "path:value" strings for easy comparison
    existingRobotClaimSets: Map<number, Set<string>> = new Map();
    duplicateClaimRobotId: number | null = null;

    private _idpSubject: Subject<string> = new Subject<string>();

    @Input()
    robotMetadata: Permissions;

    permissionForCoverAll: RobotPermission = {
        access: [],
        kind: PermissionsKinds.PROJECT,
        namespace: NAMESPACE_ALL_PROJECTS,
    };

    permissionForCoverAllForEdit: RobotPermission;

    permissionForSystem: RobotPermission = {
        access: [],
        kind: PermissionsKinds.SYSTEM,
        namespace: NAMESPACE_SYSTEM,
    };

    permissionForSystemForEdit: RobotPermission;
    showPage3: boolean = false;
    @ViewChild('wizard') wizard: ClrWizard;
    constructor(
        private configService: ConfigurationService,
        private idpService: FederatedIdpService,
        private robotService: RobotService,
        private msgHandler: MessageHandlerService,
        private operationService: OperationService
    ) {}
    ngOnInit(): void {
        this.subscribeName();
        // this.subscribeIdp();
        this.fetchIdps();
        this.claims = [
            {
                path: '',
                value: '',
            },
        ];
    }
    ngOnDestroy() {
        if (this._nameSubscription) {
            this._nameSubscription.unsubscribe();
            this._nameSubscription = null;
        }
    }

    // Custom validator function to check if selection is in idpNames
    isValidIdp(value: string): boolean {
        return this.idpNames.includes(value);
    }

    fetchIdps() {
        console.log('[fetchIdps] Fetching federated IdPs...');
        this.idpNames = [];
        this.loadingIdps = true;
        this.idpService.ListFederatedIdps({}).subscribe(
            res => {
                console.log('[fetchIdps] Fetched Idps: ', res);
                this.idpNames = res.map(idp => idp.name);
                this.idpMap = new Map(
                    res.map(idp => [idp.name.trim().toLowerCase(), idp.id])
                );
                this.loadingIdps = false;
            },
            error => {
                console.error('[fetchIdps] Error fetching IdPs:', error);
                this.idpNames = [];
            }
        );
    }

    // Trigger this when user types or changes selection
    onIdpChange(selectedIdp: string) {
        console.log(`[onIdpChange] User typed or selected: "${selectedIdp}"`);
        if (this.isValidIdp(selectedIdp)) {
            this.idpSelection = selectedIdp;
            this.fetchInheritedClaims(selectedIdp);
        } else {
            this.idpSelection = '';
        }
        // this._idpSubject.next(value);
    }

    fetchInheritedClaims(idpName: string) {
        console.log('[fetchInheritedClaims] Fetching inherited claims...');
        console.log(
            '[fetchInheritedClaims] what is the idpName...',
            idpName.trim()
        );
        console.log('[fetchInheritedClaims] idpMap...', this.idpMap);
        console.log('[fetchInheritedClaims] idpName:', JSON.stringify(idpName));
        console.log(
            '[fetchInheritedClaims] keys:',
            Array.from(this.idpMap.keys())
        );
        // In real case, replace with API call returning claims
        // const idpID: number = this.idpMap[idpName.trim()];
        const idpID = this.idpMap.get(idpName.trim().toLowerCase());
        console.log('[fetchInheritedClaims] idpID:', idpID);

        // First fetch the IDP to get claims_supported
        this.idpService.GetFederatedIdp({ id: idpID }).subscribe(
            idp => {
                this.claimsSupported = idp.claims_supported || [];
                console.log(
                    '[fetchInheritedClaims] claimsSupported:',
                    this.claimsSupported
                );
            },
            error => {
                console.error(
                    '[fetchInheritedClaims] Error fetching IDP:',
                    error
                );
                this.claimsSupported = [];
            }
        );

        this.idpService.ListClaimRules({ id: idpID }).subscribe(
            claimRules => {
                console.log('[fetchInheritedClaims] claimRules:', claimRules);

                // Build inherited claims (claims not tied to any robot)
                const claims = claimRules
                    .filter(c => c.robot_id === 0 || c.robot_id == null)
                    .map(c => ({
                        path: c.claim_path,
                        value: c.value,
                    }));
                this.inheritedClaims = claims;

                // Build existing robot claim sets for duplicate detection
                this.buildExistingRobotClaimSets(claimRules);
            },
            error => {
                this.saveBtnState = ClrLoadingState.ERROR;
                this.inlineAlertComponent.showInlineError(error);
            }
        );
    }

    /**
     * Build a map of existing robot claim sets from all claim rules.
     * Each robot's claims are stored as a Set of "path:value" strings for easy comparison.
     */
    buildExistingRobotClaimSets(claimRules: any[]) {
        this.existingRobotClaimSets.clear();
        this.duplicateClaimRobotId = null;

        // Group claims by robot_id (excluding IDP-level claims with robot_id 0 or null)
        for (const rule of claimRules) {
            if (rule.robot_id && rule.robot_id > 0) {
                if (!this.existingRobotClaimSets.has(rule.robot_id)) {
                    this.existingRobotClaimSets.set(rule.robot_id, new Set());
                }
                // Create a normalized key for comparison (lowercase, trimmed)
                const claimKey = `${(rule.claim_path || '')
                    .trim()
                    .toLowerCase()}:${(rule.value || '').trim().toLowerCase()}`;
                this.existingRobotClaimSets.get(rule.robot_id).add(claimKey);
            }
        }

        console.log(
            '[buildExistingRobotClaimSets] Built claim sets for robots:',
            Array.from(this.existingRobotClaimSets.keys())
        );
    }

    initFederatedStateForEdit(federatedidpId: number) {
        // Find the IDP name from the idpMap by ID
        let idpName = '';
        for (const [name, id] of this.idpMap.entries()) {
            if (id === federatedidpId) {
                // idpMap keys are lowercase, find original name from idpNames
                idpName =
                    this.idpNames.find(n => n.trim().toLowerCase() === name) ||
                    name;
                break;
            }
        }

        if (idpName) {
            this.idpSelection = idpName;
            // Fetch inherited claims for this IDP
            this.idpService.ListClaimRules({ id: federatedidpId }).subscribe(
                claimRules => {
                    const robotId = this.systemRobot.id;

                    // Inherited claims are those not tied to any robot (robot_id is 0, null, or undefined)
                    this.inheritedClaims = claimRules
                        .filter(c => !c.robot_id || c.robot_id === 0)
                        .map(c => ({
                            path: c.claim_path,
                            value: c.value,
                        }));

                    // Robot-specific claims (robot_id matches current robot and is not 0/null/undefined)
                    // Using == for type-coerced comparison in case robot_id comes as string from API
                    const robotClaims = claimRules
                        .filter(
                            c =>
                                c.robot_id &&
                                c.robot_id > 0 &&
                                c.robot_id == robotId
                        )
                        .map(c => ({
                            path: c.claim_path,
                            value: c.value,
                        }));

                    this.claims =
                        robotClaims.length > 0
                            ? robotClaims
                            : [{ path: '', value: '' }];
                    this.initClaims = clone(this.claims);

                    // Build existing robot claim sets for duplicate detection
                    this.buildExistingRobotClaimSets(claimRules);
                },
                error => {
                    console.error(
                        '[initFederatedStateForEdit] Error fetching claims:',
                        error
                    );
                }
            );
        } else {
            // If idpMap is not yet populated, fetch IDPs first then retry
            this.idpService.ListFederatedIdps({}).subscribe(
                res => {
                    this.idpNames = res.map(idp => idp.name);
                    this.idpMap = new Map(
                        res.map(idp => [idp.name.trim().toLowerCase(), idp.id])
                    );
                    // Now find the IDP name
                    const foundIdp = res.find(idp => idp.id === federatedidpId);
                    if (foundIdp) {
                        this.idpSelection = foundIdp.name;
                        this.initFederatedStateForEdit(federatedidpId);
                    }
                },
                error => {
                    console.error(
                        '[initFederatedStateForEdit] Error fetching IdPs:',
                        error
                    );
                }
            );
        }
    }

    // Rule 1: path in claims must not duplicate inheritedClaims
    isPathDuplicate(path: string): boolean {
        return this.inheritedClaims.some(claim => claim.path === path);
    }

    hasDuplicateClaimPaths(claims: any[]): boolean {
        const seen = new Set<string>();
        for (const claim of claims) {
            const p = claim?.path.trim().toLowerCase();
            if (p && seen.has(p)) {
                claim.error = 'Duplicate claim found';
                return true;
            }
            seen.add(p);
        }
        return false;
    }

    /**
     * Validates if a claim path follows valid JWT/OIDC claim path format
     * Valid formats: "sub", "email", "user.name", "groups[0]", "org.teams[*]", "custom_claim"
     */
    isValidClaimPath(path: string): boolean {
        if (!path || path.trim() === '') {
            return false;
        }
        return CLAIM_PATH_PATTERN.test(path.trim());
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

    /**
     * Trims whitespace from a claim's path and value
     */
    trimClaim(claim: Claim): void {
        if (claim.path) {
            claim.path = claim.path.trim();
        }
        if (claim.value) {
            claim.value = claim.value.trim();
        }
    }

    /**
     * Called on blur to trim the claim field
     */
    onClaimBlur(claim: Claim): void {
        this.trimClaim(claim);
        this.isValidFinalState();
    }

    // Final state validation: must have at least one non-empty claim with valid format
    isValidFinalState(): boolean {
        // Clear previous errors
        this.claims.forEach(c => (c.error = ''));

        // Rule 1: At least one non-empty claim
        const hasUserClaims = this.claims.some(
            c => c.path.trim() !== '' && c.value.trim() !== ''
        );

        if (!hasUserClaims) {
            console.warn('Validation failed: No valid user claims.');
            return false;
        }

        // Rule 2: Validate each claim
        for (const c of this.claims) {
            const path = (c.path || '').trim();
            const value = (c.value || '').trim();

            // Skip empty claims (user might be in the middle of adding)
            if (!path && !value) {
                continue;
            }

            // Required field validation
            if (!path || !value) {
                c.error = 'Path and Value are required.';
                return false;
            }

            // Validate claim path format
            if (!this.isValidClaimPath(path)) {
                c.error =
                    'Invalid claim path format. Use letters, numbers, underscores, hyphens, or dots (e.g., "user.name").';
                return false;
            }

            // Validate claim path against claims_supported
            if (!this.isClaimPathInSupported(path)) {
                c.error = `Claim path '${path}' is not in the IdP's supported claims list`;
                return false;
            }

            // Check duplicate with inherited claims (case-insensitive)
            const isDuplicate = this.inheritedClaims?.some(
                ic => ic.path.trim().toLowerCase() === path.toLowerCase()
            );

            if (isDuplicate) {
                c.error = 'Duplicate claim path found in inherited claims.';
                return false;
            }

            // Clear previous error if OK
            c.error = '';
        }

        // Rule 3: No duplicate keys in user claims
        if (this.hasDuplicateClaimPaths(this.claims)) {
            console.error('Please remove duplicates in claims.');
            return false;
        }

        // Rule 4: Check if claim set matches another robot's claims exactly
        const duplicateRobotId = this.findDuplicateClaimSet();
        if (duplicateRobotId !== null) {
            this.duplicateClaimRobotId = duplicateRobotId;
            console.error(
                `[isValidFinalState] Duplicate claim set found with robot ID: ${duplicateRobotId}`
            );
            return false;
        }
        this.duplicateClaimRobotId = null;

        return true;
    }

    /**
     * Check if the current claims exactly match any existing robot's claim set.
     * Returns the robot_id of the matching robot, or null if no match found.
     *
     * Two robots cannot have identical claim sets because it would make it
     * ambiguous which robot a JWT token should authenticate as.
     */
    findDuplicateClaimSet(): number | null {
        // Get non-empty claims from current input
        const currentClaims = this.claims.filter(
            c => c.path.trim() !== '' && c.value.trim() !== ''
        );

        // If no claims, no duplicate check needed
        if (currentClaims.length === 0) {
            return null;
        }

        // Build current claim set as a Set of normalized "path:value" strings
        const currentClaimSet = new Set<string>();
        for (const claim of currentClaims) {
            const claimKey = `${claim.path.trim().toLowerCase()}:${claim.value
                .trim()
                .toLowerCase()}`;
            currentClaimSet.add(claimKey);
        }

        console.log(
            '[findDuplicateClaimSet] Current claim set:',
            Array.from(currentClaimSet)
        );

        // Compare against each existing robot's claim set
        const entries = Array.from(this.existingRobotClaimSets.entries());
        for (let i = 0; i < entries.length; i++) {
            const [robotId, existingClaimSet] = entries[i];
            // In edit mode, skip the robot being edited
            if (this.isEditMode && this.originalRobotForEdit?.id === robotId) {
                continue;
            }

            // Check if sets are exactly equal (same size and same elements)
            if (this.areSetsEqual(currentClaimSet, existingClaimSet)) {
                console.log(
                    `[findDuplicateClaimSet] Found duplicate with robot ID: ${robotId}`
                );
                return robotId;
            }
        }

        return null;
    }

    /**
     * Check if two sets have exactly the same elements
     */
    areSetsEqual(setA: Set<string>, setB: Set<string>): boolean {
        if (setA.size !== setB.size) {
            return false;
        }
        const items = Array.from(setA);
        for (let i = 0; i < items.length; i++) {
            if (!setB.has(items[i])) {
                return false;
            }
        }
        return true;
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
                                q: encodeURIComponent(`name=${name}`),
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
        return this.systemRobot.duration < -1;
    }
    inputExpiration() {
        if (+this.systemRobot.duration === -1) {
            this.expirationType = ExpirationType.NEVER;
        } else {
            this.expirationType = ExpirationType.DAYS;
        }
    }
    changeExpirationType() {
        if (this.expirationType === ExpirationType.DEFAULT) {
            this.systemRobot.duration = this.systemExpirationDays;
        }
        if (this.expirationType === ExpirationType.DAYS) {
            this.systemRobot.duration = this.systemExpirationDays;
        }
        if (this.expirationType === ExpirationType.NEVER) {
            this.systemRobot.duration = -1;
        }
    }
    getSystemRobotExpiration() {
        this.loadingSystemConfig = true;
        this.configService
            .getConfiguration()
            .pipe(finalize(() => (this.loadingSystemConfig = false)))
            .subscribe(res => {
                if (
                    res &&
                    res.robot_token_duration &&
                    res.robot_token_duration.value
                ) {
                    this.systemRobot.duration = res.robot_token_duration.value;
                    this.systemExpirationDays = this.systemRobot.duration;
                }
            });
    }
    inputName() {
        this._nameSubject.next(this.systemRobot.name);
    }
    cancel() {
        this.wizard.reset();
        this.reset();
        this.addRobotOpened = false;
    }

    reset() {
        this.open(false);
        this.systemRobot = clone(NEW_EMPTY_ROBOT);
        this.permissionForCoverAll = {
            access: [],
            kind: PermissionsKinds.PROJECT,
            namespace: NAMESPACE_ALL_PROJECTS,
        };
        this.permissionForSystem = {
            access: [],
            kind: PermissionsKinds.SYSTEM,
            namespace: NAMESPACE_SYSTEM,
        };
        this.coverAll = false;
        this.showPage3 = false;
        this.robotForm.reset();
        this.expirationType = ExpirationType.DAYS;
        this.getSystemRobotExpiration();
        // Reset federated robot state
        this.useFederatedRobot = false;
        this.idpSelection = '';
        this.claims = [{ path: '', value: '' }];
        this.inheritedClaims = [];
        this.existingRobotClaimSets.clear();
        this.duplicateClaimRobotId = null;
    }
    resetForEdit(robot: Robot) {
        this.open(true);
        this.originalRobotForEdit = clone(robot);
        this.systemRobot = clone(robot);
        this.permissionForSystem = {
            access: getSystemAccess(robot),
            kind: PermissionsKinds.SYSTEM,
            namespace: NAMESPACE_SYSTEM,
        };

        this.permissionForSystemForEdit = clone(this.permissionForSystem);

        this.expirationType =
            robot.duration === -1 ? ExpirationType.NEVER : ExpirationType.DAYS;
        if (robot && robot.permissions && robot.permissions.length) {
            this.coverAll = false;
            robot.permissions.forEach(item => {
                if (
                    item.kind === PermissionsKinds.PROJECT &&
                    item.namespace === NAMESPACE_ALL_PROJECTS
                ) {
                    this.coverAll = true;
                    this.permissionForCoverAll = clone(item);
                    this.permissionForCoverAllForEdit = clone(item);
                }
            });
        }
        console.log('robot: ', robot);

        // Initialize federated robot state based on federatedidp_id
        const hasFederatedIdp =
            robot.federatedidp_id && robot.federatedidp_id > 0;
        this.useFederatedRobot = hasFederatedIdp;
        if (hasFederatedIdp) {
            this.initFederatedStateForEdit(robot.federatedidp_id);
        }

        this.robotForm.reset({
            name: this.systemRobot.name,
            expiration: this.systemRobot.duration,
            description: this.systemRobot.description,
            useFederatedRobotAccount: this.useFederatedRobot,
        });
        this.coverAllForEdit = this.coverAll;
    }
    open(isEditMode: boolean) {
        this.isNameExisting = false;
        this.isEditMode = isEditMode;
        this.addRobotOpened = true;
        this.inlineAlertComponent.close();
        this._nameSubject.next('');
    }
    disabled(): boolean {
        if (!this.isEditMode) {
            return !this.canAdd();
        }
        return !this.canEdit();
    }
    canAdd(): boolean {
        if (this.robotForm.invalid) {
            return false;
        }
        if (this.coverAll) {
            if (!this.permissionForCoverAll.access?.length) {
                return false;
            }
        } else {
            if (
                !this.permissionForSystem?.access?.length &&
                !this.listAllProjectsComponent?.selectedRow?.length
            ) {
                return false;
            }
            if (this.listAllProjectsComponent?.selectedRow?.length) {
                for (
                    let i = 0;
                    i < this.listAllProjectsComponent?.selectedRow?.length;
                    i++
                ) {
                    if (
                        !this.listAllProjectsComponent
                            ?.selectedProjectPermissionMap[
                            this.listAllProjectsComponent?.selectedRow[i].name
                        ]?.length
                    ) {
                        return false;
                    }
                }
            }
        }
        return true;
    }
    canEdit() {
        if (!this.canAdd()) {
            return false;
        }
        // eslint-disable-next-line eqeqeq
        if (this.systemRobot.duration != this.originalRobotForEdit.duration) {
            return true;
        }
        // eslint-disable-next-line eqeqeq
        if (
            this.systemRobot.description !=
            this.originalRobotForEdit.description
        ) {
            return true;
        }
        if (
            !isSameObject(
                this.permissionForSystem,
                this.permissionForSystemForEdit
            )
        ) {
            return true;
        }
        if (this.coverAll !== this.coverAllForEdit) {
            return true;
        }
        if (this.coverAll) {
            if (
                !isSameObject(
                    this.permissionForCoverAll,
                    this.permissionForCoverAllForEdit
                )
            ) {
                return true;
            }
        }
        if (this.listAllProjectsComponent) {
            if (
                !isSameArrayValue(
                    this.listAllProjectsComponent.selectedRow,
                    this.listAllProjectsComponent.selectedRowForEdit
                )
            ) {
                return true;
            } else {
                for (
                    let i = 0;
                    i < this.listAllProjectsComponent.selectedRow.length;
                    i++
                ) {
                    if (
                        !isSameArrayValue(
                            this.listAllProjectsComponent
                                .selectedProjectPermissionMap[
                                this.listAllProjectsComponent.selectedRow[i]
                                    .name
                            ],
                            this.listAllProjectsComponent
                                .selectedProjectPermissionMapForEdit[
                                this.listAllProjectsComponent.selectedRow[i]
                                    .name
                            ]
                        )
                    ) {
                        return true;
                    }
                }
            }
        }
        // Check for claims changes in federated robot
        if (this.useFederatedRobot && this.hasClaimsChanged()) {
            return true;
        }
        return false;
    }

    hasClaimsChanged(): boolean {
        if (!this.initClaims || !this.claims) {
            return false;
        }
        // Filter out empty claims for comparison
        const currentClaims = this.claims.filter(
            c => c.path.trim() !== '' && c.value.trim() !== ''
        );
        const originalClaims = this.initClaims.filter(
            c => c.path.trim() !== '' && c.value.trim() !== ''
        );

        if (currentClaims.length !== originalClaims.length) {
            return true;
        }

        // Compare each claim
        for (let i = 0; i < currentClaims.length; i++) {
            const current = currentClaims[i];
            const original = originalClaims.find(
                o => o.path.trim() === current.path.trim()
            );
            if (!original || original.value.trim() !== current.value.trim()) {
                return true;
            }
        }

        return false;
    }

    getClaimsChanges(): {
        claimsToAdd: Claim[];
        claimsToDelete: Claim[];
    } {
        const claimsToAdd: Claim[] = [];
        const claimsToDelete: Claim[] = [];

        // Filter out empty claims
        const currentClaims = this.claims.filter(
            c => c.path.trim() !== '' && c.value.trim() !== ''
        );
        const initClaims = (this.initClaims || []).filter(
            c => c.path.trim() !== '' && c.value.trim() !== ''
        );

        // Return early if both are empty
        if (!currentClaims.length && !initClaims.length) {
            return { claimsToAdd, claimsToDelete };
        }

        // Convert both arrays into Map for O(1) lookup by path
        const currentMap = new Map(
            currentClaims.map(c => [c.path.trim(), c.value.trim()])
        );
        const initMap = new Map(
            initClaims.map(c => [c.path.trim(), c.value.trim()])
        );

        // Iterate over initial claims to detect deletions or modifications
        for (const [path, oldValue] of initMap.entries()) {
            const newValue = currentMap.get(path);

            if (newValue === undefined) {
                // Claim removed -> delete
                claimsToDelete.push({ path, value: oldValue });
            } else if (oldValue !== newValue) {
                // Modified -> delete old + add new
                claimsToDelete.push({ path, value: oldValue });
                claimsToAdd.push({ path, value: newValue });
            }
        }

        // Detect newly added claims
        for (const [path, newValue] of currentMap.entries()) {
            if (!initMap.has(path)) {
                // New claim added
                claimsToAdd.push({ path, value: newValue });
            }
        }

        return { claimsToAdd, claimsToDelete };
    }

    assembleClaimsToAddRules(
        claimsToAdd: Claim[],
        fedidpId: number,
        robotId: number
    ) {
        if (
            !claimsToAdd ||
            !Array.isArray(claimsToAdd) ||
            claimsToAdd.length === 0
        ) {
            return [];
        }
        return claimsToAdd
            .filter(claim => claim.path?.trim() && claim.value?.trim())
            .map(claim => ({
                claim_path: claim.path.trim(),
                value: claim.value.trim(),
                identity_provider_id: fedidpId,
                robot_id: robotId,
            }));
    }

    assembleClaimsToDeleteRules(
        claimsToDelete: Claim[],
        fedidpId: number,
        robotId: number
    ) {
        if (
            !claimsToDelete ||
            !Array.isArray(claimsToDelete) ||
            claimsToDelete.length === 0
        ) {
            return [];
        }
        return claimsToDelete
            .filter(claim => claim.path?.trim() && claim.value?.trim())
            .map(claim => ({
                claim_path: claim.path.trim(),
                value: claim.value.trim(),
                identity_provider_id: fedidpId,
                robot_id: robotId,
            }));
    }

    updateClaimsForEdit(robot: Robot, opeMessage: OperateInfo) {
        const { claimsToAdd, claimsToDelete } = this.getClaimsChanges();
        const fedidpId = robot.federatedidp_id;
        const robotId = this.originalRobotForEdit.id;

        const deletePayload = this.assembleClaimsToDeleteRules(
            claimsToDelete,
            fedidpId,
            robotId
        );
        const addPayload = this.assembleClaimsToAddRules(
            claimsToAdd,
            fedidpId,
            robotId
        );

        // First delete claims, then add new ones
        if (deletePayload.length > 0) {
            this.idpService
                .DeleteClaimRules({
                    id: fedidpId,
                    claims: { rules: deletePayload },
                })
                .subscribe(
                    () => {
                        // After successful delete, add new claims
                        if (addPayload.length > 0) {
                            this.createClaimsAfterDelete(
                                fedidpId,
                                addPayload,
                                opeMessage
                            );
                        } else {
                            this.finishEditSuccess(opeMessage);
                        }
                    },
                    error => {
                        console.error('Error deleting claim rules:', error);
                        this.saveBtnState = ClrLoadingState.ERROR;
                        operateChanges(
                            opeMessage,
                            OperationState.failure,
                            errorHandler(error)
                        );
                        this.inlineAlertComponent.showInlineError(error);
                    }
                );
        } else if (addPayload.length > 0) {
            // No claims to delete, just add new ones
            this.createClaimsAfterDelete(fedidpId, addPayload, opeMessage);
        } else {
            // No claims changes
            this.finishEditSuccess(opeMessage);
        }
    }

    createClaimsAfterDelete(
        fedidpId: number,
        addPayload: any[],
        opeMessage: OperateInfo
    ) {
        this.idpService
            .CreateClaimRules({
                id: fedidpId,
                claims: { rules: addPayload },
            })
            .subscribe(
                () => {
                    this.finishEditSuccess(opeMessage);
                },
                error => {
                    console.error('Error creating claim rules:', error);
                    this.saveBtnState = ClrLoadingState.ERROR;
                    operateChanges(
                        opeMessage,
                        OperationState.failure,
                        errorHandler(error)
                    );
                    this.inlineAlertComponent.showInlineError(error);
                }
            );
    }

    finishEditSuccess(opeMessage: OperateInfo) {
        this.saveBtnState = ClrLoadingState.SUCCESS;
        this.addSuccess.emit(null);
        this.cancel();
        operateChanges(opeMessage, OperationState.success);
        this.msgHandler.showSuccess('SYSTEM_ROBOT.UPDATE_ROBOT_SUCCESSFULLY');
    }

    assembleClaimRules(claims: Claim[], fedidp_id: number, robot_id: number) {
        if (!claims || !Array.isArray(claims)) {
            console.error("Input 'claims' is not a valid array.");
            return [];
        }
        if (robot_id === 0 || robot_id === undefined) {
            console.error(
                "Assembling claim rules: Input is missing an 'robot_id' property."
            );
            return [];
        }
        if (fedidp_id === 0 || fedidp_id === undefined) {
            console.error(
                "Assembling claim rules: Input is missing an 'fedidp_id' property."
            );
            return [];
        }

        // Filter out empty claims and trim values before sending to API
        return claims
            .filter(claim => claim.path?.trim() && claim.value?.trim())
            .map(claim => ({
                claim_path: claim.path.trim(),
                value: claim.value.trim(),
                identity_provider_id: fedidp_id,
                robot_id: robot_id,
            }));
    }

    save() {
        const robot: Robot = clone(this.systemRobot);
        robot.disable = false;
        robot.level = PermissionsKinds.SYSTEM;
        robot.duration = +this.systemRobot.duration;
        if (this.useFederatedRobot) {
            robot.federatedidp_id = this.idpMap.get(
                this.idpSelection.trim().toLowerCase()
            );
        }

        if (
            robot.federatedidp_id === undefined ||
            robot.federatedidp_id === null ||
            robot.federatedidp_id === 0
        ) {
            if (!this.useFederatedRobot) {
                robot.federatedidp_id = 0;
            } else {
                console.error('Robot.federatedidp_id is undefined or null');
                this.inlineAlertComponent.showInlineError(
                    'SYSTEM_ROBOT.FEDERATED_IDP_NOT_FOUND'
                );
                return;
            }
        }
        robot.permissions = [];
        if (this.permissionForSystem?.access?.length) {
            robot.permissions.push(this.permissionForSystem);
        }
        if (this.coverAll) {
            if (this.permissionForCoverAll?.access?.length) {
                robot.permissions.push(this.permissionForCoverAll);
            }
        } else {
            this.listAllProjectsComponent.selectedRow.forEach(item => {
                if (
                    this.listAllProjectsComponent.selectedProjectPermissionMap[
                        item.name
                    ]?.length
                ) {
                    robot.permissions.push({
                        kind: PermissionsKinds.PROJECT,
                        namespace: item.name,
                        access: this.listAllProjectsComponent
                            .selectedProjectPermissionMap[item.name],
                    });
                }
            });
        }
        // Push permission must work with pull permission
        if (robot.permissions && robot.permissions.length) {
            for (let i = 0; i < robot.permissions.length; i++) {
                if (onlyHasPushPermission(robot.permissions[i].access)) {
                    this.inlineAlertComponent.showInlineError(
                        'SYSTEM_ROBOT.PUSH_PERMISSION_TOOLTIP'
                    );
                    return;
                }
            }
        }
        this.saveBtnState = ClrLoadingState.LOADING;
        if (this.isEditMode) {
            robot.disable = this.systemRobot.disable;
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
                        // Handle claims update for federated robots
                        if (this.useFederatedRobot && this.hasClaimsChanged()) {
                            this.updateClaimsForEdit(robot, opeMessage);
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
            opeMessage.data.name = robot.name;
            this.operationService.publishInfo(opeMessage);
            this.robotService
                .CreateRobot({
                    robot: robot,
                })
                .subscribe(
                    res => {
                        console.log('created robot acc resp: ', res);
                        if (this.useFederatedRobot) {
                            console.log('going to start the claims addition');
                            const assembledClaimRules = this.assembleClaimRules(
                                this.claims,
                                robot.federatedidp_id,
                                res.id
                            );
                            console.log(
                                'assembled claim rules: ',
                                assembledClaimRules
                            );
                            // Store the created robot response to use after claims are created
                            const createdRobot = res;
                            this.idpService
                                .CreateClaimRules({
                                    id: robot.federatedidp_id,
                                    claims: {
                                        rules: assembledClaimRules,
                                    },
                                })
                                .subscribe(
                                    () => {
                                        this.saveBtnState =
                                            ClrLoadingState.SUCCESS;
                                        // Emit the created robot (with id) after claims are created
                                        this.addSuccess.emit(createdRobot);
                                        this.cancel();
                                        operateChanges(
                                            opeMessage,
                                            OperationState.success
                                        );
                                    },
                                    error => {
                                        this.saveBtnState =
                                            ClrLoadingState.ERROR;
                                        this.inlineAlertComponent.showInlineError(
                                            error
                                        );
                                        operateChanges(
                                            opeMessage,
                                            OperationState.failure,
                                            errorHandler(error)
                                        );
                                    }
                                );
                            // Return early for federated robots - emit happens after claims are created
                            return;
                        }
                        this.saveBtnState = ClrLoadingState.SUCCESS;
                        this.addSuccess.emit(res);
                        this.cancel();
                        operateChanges(opeMessage, OperationState.success);
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

    calculateExpiresAt(): Date {
        if (
            this.systemRobot &&
            this.systemRobot.creation_time &&
            this.systemRobot.duration > 0
        ) {
            return new Date(
                new Date(this.systemRobot.creation_time).getTime() +
                    this.systemRobot.duration * MINI_SECONDS_ONE_DAY
            );
        }
        return null;
    }
    shouldShowWarning(): boolean {
        return new Date() >= this.calculateExpiresAt();
    }

    clrWizardPageOnLoad() {
        this.inlineAlertComponent.close();
        this.showPage3 = true;
    }

    // Function to add a new claim pair
    addClaim(): void {
        this.claims.push({ path: '', value: '' });
    }

    deleteClaim(index: number): void {
        if (this.claims.length === 1) {
            this.claims = [{ path: '', value: '' }];
            return;
        }
        this.claims.splice(index, 1);
    }

    protected readonly PermissionSelectPanelModes = PermissionSelectPanelModes;
}
