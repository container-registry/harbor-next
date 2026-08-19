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
    AfterViewChecked,
    Component,
    EventEmitter,
    OnDestroy,
    OnInit,
    Output,
    ViewChild,
} from '@angular/core';
import { NgForm } from '@angular/forms';
import { Subscription, throwError as observableThrowError } from 'rxjs';
import { TranslateService } from '@ngx-translate/core';
import { ErrorHandler } from '../../../../shared/units/error-handler';
import { InlineAlertComponent } from '../../../../shared/components/inline-alert/inline-alert.component';
import {
    clone,
    compareValue,
    CURRENT_BASE_HREF,
    isEmptyObject,
} from '../../../../shared/units/utils';
import { HttpClient } from '@angular/common/http';
import { catchError } from 'rxjs/operators';
import { AppConfigService } from '../../../../services/app-config.service';
import { ClrLoadingState } from '@clr/angular';
import { FederatedIdp, FederatedIdpUpdate } from 'ng-swagger-gen/models';
import { FederatedIdpService } from 'ng-swagger-gen/services';
import { SystemInfo } from 'src/app/shared/services';
import { id } from '@cds/core/internal';

// const FAKE_JSON_KEY = 'No Change';
// const METADATA_URL = CURRENT_BASE_HREF + '/replication/adapterinfos';
const FIXED_PATTERN_TYPE: string = 'EndpointPatternTypeFix';

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
    error?: string; // optional validation error message
}

@Component({
    selector: 'hbr-create-edit-idp',
    templateUrl: './create-edit-idp.component.html',
    styleUrls: ['./create-edit-idp.component.scss'],
})
export class CreateEditIdpComponent
    implements AfterViewChecked, OnDestroy, OnInit
{
    modalTitle: string;
    // urlDisabled: boolean = false;
    editDisabled: boolean = false;
    createEditIdpOpened: boolean;
    staticBackdrop: boolean = true;
    closable: boolean = false;
    editable: boolean;
    target: FederatedIdp = this.initIdp();
    openIDConfigJSON: string;
    jwksKeys: string;
    jwksError: string;
    claimsSupported: string;
    initVal: FederatedIdp;
    targetForm: NgForm;
    @ViewChild('targetForm') currentForm: NgForm;
    testOngoing: boolean;
    onGoing: boolean;
    idpId: number | string;
    systemInfo: SystemInfo;

    @ViewChild(InlineAlertComponent) inlineAlert: InlineAlertComponent;

    @Output() reload = new EventEmitter<boolean>();
    // Array to store claim data
    claims: Claim[];
    initClaims: Claim[];
    valueChangesSub: Subscription;
    formValues: { [key: string]: string } | any;
    adapterInfo: object;
    showEndpointList: boolean = false;
    endpointOnHover: boolean = false;
    testButtonState: ClrLoadingState = ClrLoadingState.DEFAULT;
    okButtonState: ClrLoadingState = ClrLoadingState.DEFAULT;

    constructor(
        private idpService: FederatedIdpService,
        // private idpDefaultService: FederatedIdpDefaultService,
        private errorHandler: ErrorHandler,
        private translateService: TranslateService,
        private http: HttpClient,
        private appConfigService: AppConfigService
    ) {}

    ngOnInit(): void {
        this.initClaims = [
            {
                path: 'iss',
                value: '',
            },
        ];
        this.claims = [
            {
                path: 'iss',
                value: '',
            },
        ];
        return;
        // this.getAdapters();
        // this.getAdapterInfo();
    }

    selectedEndpoint(endpoint: string) {
        this.targetForm.controls.endpointUrl.reset(endpoint);
        this.showEndpointList = false;
        this.endpointOnHover = false;
    }

    public get registryUrl(): string {
        return this.systemInfo ? this.systemInfo.registry_url : '';
    }

    updateClaimsSupported(): boolean {
        let isvalid = false;
        if (!this.openIDConfigJSON) return false;

        try {
            // Attempt to parse the string into an object
            const configObject = JSON.parse(this.openIDConfigJSON);
            // logic to update claims based on configObject
            // console.log('Valid JSON:', configObject);

            if (configObject.issuer) {
                isvalid = true;
                this.target.issuer = configObject.issuer;
                const existingIndex = this.claims.findIndex(
                    c => c.path === 'iss'
                );

                if (existingIndex !== -1) {
                    // If found, update the value only
                    this.claims[existingIndex].value = this.target.issuer;
                } else {
                    // If not found, push the new object
                    this.claims.push({
                        path: 'iss',
                        value: this.target.issuer,
                    });
                }
            } else {
                isvalid = false;
                this.inlineAlert.showInlineError(
                    'Invalid OpenID Config: Issuer not found.'
                );
                this.target.issuer = '';
                const existingIndex = this.claims.findIndex(
                    c => c.path === 'iss'
                );

                if (existingIndex !== -1) {
                    // If found, update the value only
                    this.claims[existingIndex].value = '';
                } else {
                    // If not found, push the new object
                    this.claims.push({
                        path: 'iss',
                        value: '',
                    });
                }
            }

            if (configObject.jwks_uri) {
                this.target.jwks_uri = configObject.jwks_uri;
            } else {
                this.target.jwks_uri = '';
            }

            if (configObject.claims_supported) {
                this.claimsSupported = configObject.claims_supported.join(', ');
                this.target.claims_supported = configObject.claims_supported;
            } else {
                this.claimsSupported = '';
                this.target.claims_supported = [];
            }

            if (configObject.id_token_signing_alg_values_supported) {
                this.target.supported_algorithms =
                    configObject.id_token_signing_alg_values_supported;
            } else {
                this.target.supported_algorithms = [];
            }
        } catch (e) {
            isvalid = false;
            // Handle invalid JSON gracefully
            // console.error('Invalid JSON format');
            this.inlineAlert.showInlineError(
                'Invalid JSON format for OpenID Config.'
            );
        }

        return isvalid;
    }

    /**
     * Fetches the OpenID Configuration JSON from the provided URL
     * and stores it as a formatted string in openIDConfigJSON.
     */
    fetchOpenIDConfig() {
        if (this.target.offline_validation) {
            return;
        }
        const url = (this.target?.openid_config_url || '').trim();

        // Validate URL strictly before sending request
        if (!url || !url.startsWith('http')) {
            console.warn('⚠️ Invalid OpenID Configuration URL');
            this.inlineAlert.showInlineError(
                'Please provide a valid OpenID Configuration URL.'
            );
            return;
        }

        if (!url.includes('.well-known/openid-configuration')) {
            console.warn(
                '⚠️ Provided URL does not look like an OpenID configuration endpoint.'
            );
            this.inlineAlert.showInlineError(
                'URL must point to a valid ".well-known/openid-configuration" endpoint.'
            );
            return;
        }

        // Update UI loading state
        this.testOngoing = true;
        this.testButtonState = ClrLoadingState.LOADING;
        this.openIDConfigJSON = ''; // clear previous data

        // Call backend via service
        this.idpService
            .PingFederatedIdpOpenIDConfig({
                openidConfigUrl: { openid_config_url: url },
            })
            .subscribe(
                openIDConfigJSON => {
                    // Success callback
                    this.openIDConfigJSON = JSON.stringify(
                        openIDConfigJSON,
                        null,
                        2
                    );

                    console.log(
                        '✅ OpenID Configuration fetched successfully:',
                        openIDConfigJSON
                    );

                    // Extract the 'issuer' key and assign to target
                    if (openIDConfigJSON && openIDConfigJSON.issuer) {
                        this.target.issuer = openIDConfigJSON.issuer;
                        const existingIndex = this.claims.findIndex(
                            c => c.path === 'iss'
                        );
                        if (existingIndex !== -1) {
                            // If found, update the value only
                            this.claims[existingIndex].value =
                                this.target.issuer;
                        } else {
                            // If not found, push the new object
                            this.claims.push({
                                path: 'iss',
                                value: this.target.issuer,
                            });
                        }
                    }

                    // Extract the 'jwks_uri' key and assign to target
                    if (openIDConfigJSON && openIDConfigJSON.jwks_uri) {
                        this.target.jwks_uri = openIDConfigJSON.jwks_uri;
                    }

                    // get the jwks keys
                    // Call backend via service
                    this.idpService
                        .PingFederatedIdpJWKS({
                            jwks: { jwks_uri: this.target.jwks_uri },
                        })
                        .subscribe(
                            jwksKeys => {
                                // Success callback
                                this.jwksKeys = JSON.stringify(
                                    jwksKeys,
                                    null,
                                    2
                                );

                                console.log(
                                    '✅ JWKS Keys fetched successfully:',
                                    jwksKeys
                                );

                                if (!this.updateClaimsSupported()) {
                                    // Error callback
                                    console.error(
                                        '❌ Invalid OpenID Configuration: Unable to update Claims Supported.'
                                    );

                                    const message =
                                        'Invalid OpenID Configuration: Unable to update Claims Supported.';

                                    this.inlineAlert.showInlineError(message);
                                    this.openIDConfigJSON =
                                        'Invalid OpenID Configuration';
                                }
                            },
                            error => {
                                // Error callback
                                console.error(
                                    '❌ Failed to fetch JWKS Keys:',
                                    error
                                );

                                const message =
                                    error?.status === 404
                                        ? 'JWKS Keys not found at the provided URL.'
                                        : 'Failed to fetch JWKS Keys. Please verify the URL and network access.';

                                this.inlineAlert.showInlineError(message);
                                this.jwksKeys = 'Error fetching JWKS Keys';
                            },
                            () => {
                                // Complete callback (optional)
                                this.testOngoing = false;
                                this.testButtonState = ClrLoadingState.DEFAULT;
                            }
                        );

                    // Optional UI alerts
                    // this.inlineAlert.showInlineSuccess({
                    //     message: 'FEDERATED_IDPS.OPENIDCONFIG_FETCH_SUCCESS',
                    // });
                },
                error => {
                    // Error callback
                    console.error(
                        '❌ Failed to fetch OpenID Configuration:',
                        error
                    );

                    const message =
                        error?.status === 404
                            ? 'OpenID Configuration not found at the provided URL.'
                            : 'Failed to fetch OpenID Configuration. Please verify the URL and network access.';

                    this.inlineAlert.showInlineError(message);
                    this.openIDConfigJSON =
                        'Error fetching OpenID Configuration';
                },
                () => {
                    // Complete callback (optional)
                    this.testOngoing = false;
                    this.testButtonState = ClrLoadingState.DEFAULT;
                }
            );
    }

    blur() {
        if (!this.endpointOnHover) {
            this.showEndpointList = false;
        }
    }

    validateFedIdpName(name: string): boolean {
        // same regex pattern as Go version
        const federatedIdpName = /^[a-z0-9]+(?:[._-][a-z0-9]+)*$/;
        // returns true if valid, false otherwise (instead of throwing error)
        return federatedIdpName.test(name);
    }

    public get isValid(): boolean {
        let nametrim: string;
        let issuertrim: string;
        if (!this.target.name) {
            return false;
        } else {
            nametrim = this.target.name.trim();
            if (nametrim.length === 0) {
                this.inlineAlert.showInlineError(
                    'Invalid Federated IDP name. Federated IDP name should not contain spaces.'
                );
                return false;
            }
            if (this.target.name.length !== nametrim.length) {
                this.inlineAlert.showInlineError(
                    'Invalid Federated IDP name. Federated IDP name should not contain spaces.'
                );
                return false;
            }
            if (!this.validateFedIdpName(nametrim)) {
                this.inlineAlert.showInlineError(
                    'Invalid Federated IDP name. Federated IDP name is not in lower case or contains illegal characters.'
                );
                return false;
            }
            if (nametrim.length > 64) {
                this.inlineAlert.showInlineError(
                    'Invalid Federated IDP name. Name must not exceed 64 characters.'
                );
                return false;
            }
        }
        if (!this.target.issuer) {
            return false;
        } else {
            issuertrim = this.target.issuer.trim();
            if (issuertrim.length === 0) {
                return false;
            }
        }

        if (this.target.offline_validation && !this.isJwksKeysValid()) {
            return false;
        }

        if (this.target.offline_validation && !this.isOpenIDConfigValid()) {
            return false;
        }

        // Validate claims - must be valid and no empty claims
        if (!this.areClaimsValid()) {
            return false;
        }

        // Check basic form validity and not in progress
        if (
            this.testOngoing ||
            this.onGoing ||
            !this.targetForm ||
            !this.targetForm.valid ||
            !this.editable
        ) {
            return false;
        }

        // For new IDP (create mode), allow if form is valid
        if (!this.idpId) {
            return true;
        }

        // For edit mode, require that something has changed
        const targetChanged = !compareValue(this.target, this.initVal);
        const claimsChanged = this.haveClaimsChanged();

        return targetChanged || claimsChanged;
    }

    /**
     * Compares current claims with initial claims to detect changes.
     * Only compares path and value, ignoring the error property.
     * Also filters out empty claims before comparison.
     */
    haveClaimsChanged(): boolean {
        // Filter out empty claims for comparison
        const currentClaims = (this.claims || [])
            .filter(c => c.path?.trim() || c.value?.trim())
            .map(c => ({
                path: (c.path || '').trim(),
                value: (c.value || '').trim(),
            }));

        const initialClaims = (this.initClaims || [])
            .filter(c => c.path?.trim() || c.value?.trim())
            .map(c => ({
                path: (c.path || '').trim(),
                value: (c.value || '').trim(),
            }));

        // Different number of claims = changed
        if (currentClaims.length !== initialClaims.length) {
            return true;
        }

        // Build maps for comparison
        const currentMap = new Map<string, string>();
        currentClaims.forEach(c =>
            currentMap.set(c.path.toLowerCase(), c.value)
        );

        const initMap = new Map<string, string>();
        initialClaims.forEach(c => initMap.set(c.path.toLowerCase(), c.value));

        // Check if all paths exist and values match
        for (const [path, value] of Array.from(currentMap.entries())) {
            if (!initMap.has(path) || initMap.get(path) !== value) {
                return true;
            }
        }

        // Check reverse - any paths in init that don't exist in current
        for (const path of Array.from(initMap.keys())) {
            if (!currentMap.has(path)) {
                return true;
            }
        }

        return false;
    }

    /**
     * Checks if all claims are valid without showing inline alerts.
     * Used by isValid getter to enable/disable OK button.
     */
    areClaimsValid(): boolean {
        if (!this.claims || !Array.isArray(this.claims)) {
            return false;
        }

        // Check required claims exist (iss)
        const paths = this.claims
            .filter(c => c.path && c.path.trim())
            .map(c => c.path.trim().toLowerCase());
        const requiredKeys = ['iss'];
        const hasAllRequired = requiredKeys.every(key => paths.includes(key));
        if (!hasAllRequired) {
            return false;
        }

        // Check for duplicate paths
        if (this.hasDuplicateClaimPaths()) {
            return false;
        }

        // Check each claim is valid
        for (const claim of this.claims) {
            const path = (claim.path || '').trim();
            const value = (claim.value || '').trim();

            // Skip completely empty claims only if they are extra (not required)
            if (!path && !value) {
                continue;
            }

            // If path exists, value must exist
            if (path && !value) {
                return false;
            }

            // If value exists, path must exist
            if (value && !path) {
                return false;
            }

            // Validate path format (skip for mandatory claim iss)
            if (path && !this.isValidClaimPath(path)) {
                // Check if it's a mandatory claim (iss is always valid)
                if (path.toLowerCase() !== 'iss') {
                    return false;
                }
            }

            // Check for existing errors
            if (claim.error) {
                return false;
            }
        }

        return true;
    }

    public get inProgress(): boolean {
        return this.onGoing || this.testOngoing;
    }

    setOfflineValidation($event: any) {
        this.target.offline_validation = $event;
    }

    // Function to add a new claim pair
    addClaim(): void {
        this.claims.push({ path: '', value: '' });
    }

    deleteClaim(index: number): void {
        if (this.claims.length === 1) {
            return;
        }
        if (index === 0) {
            return;
        }
        if (this.checkIfMandotaryClaim(index)) {
            return;
        }
        this.claims.splice(index, 1);
    }

    checkIfIssuerClaim(index: number): boolean {
        if (this.claims[index].path === 'iss') {
            return true;
        }
    }

    checkIfMandotaryClaim(index: number): boolean {
        if (this.claims[index].path === 'iss') {
            return true;
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
        if (
            !this.target.claims_supported ||
            this.target.claims_supported.length === 0
        ) {
            return true;
        }
        const normalizedPath = path.trim().toLowerCase();
        return this.target.claims_supported.some(
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
     * Called on blur to trim the claim field and validate
     */
    onClaimBlur(claim: Claim): void {
        this.trimClaim(claim);
        this.validateAllClaims();
    }

    /**
     * Check for duplicate claim paths within the claims array
     */
    hasDuplicateClaimPaths(): boolean {
        const seen = new Set<string>();
        for (const claim of this.claims) {
            const p = (claim?.path || '').trim().toLowerCase();
            if (p && seen.has(p)) {
                return true;
            }
            seen.add(p);
        }
        return false;
    }

    /**
     * Find the index of a duplicate claim path
     */
    findDuplicateClaimIndex(): number {
        const seen = new Map<string, number>();
        for (let i = 0; i < this.claims.length; i++) {
            const p = (this.claims[i]?.path || '').trim().toLowerCase();
            if (p && seen.has(p)) {
                return i; // Return the second occurrence
            }
            seen.set(p, i);
        }
        return -1;
    }

    /**
     * Validates all claims and sets error messages
     * Returns true if all claims are valid
     */
    validateAllClaims(): boolean {
        // Clear previous errors
        this.claims.forEach(c => (c.error = ''));

        let isValid = true;

        for (let i = 0; i < this.claims.length; i++) {
            const claim = this.claims[i];
            const path = (claim.path || '').trim();
            const value = (claim.value || '').trim();

            // Skip completely empty claims (user might be in the middle of adding)
            if (!path && !value) {
                continue;
            }

            // Skip mandatory claims validation for path format (aud, iss are always valid)
            if (this.checkIfMandotaryClaim(i)) {
                // But still check if value is empty for mandatory claims
                if (!value) {
                    claim.error = 'Value is required';
                    isValid = false;
                    continue;
                }
                // Validate mandatory claims against claims_supported
                if (!this.isClaimPathInSupported(path)) {
                    claim.error = `Claim path '${path}' is not in the IdP's supported claims list`;
                    isValid = false;
                }
                continue;
            }

            // Required field validation
            if (!path) {
                claim.error = 'Path is required';
                isValid = false;
                continue;
            }

            if (!value) {
                claim.error = 'Value is required';
                isValid = false;
                continue;
            }

            // Validate claim path format
            if (!this.isValidClaimPath(path)) {
                claim.error =
                    'Invalid claim path format. Use letters, numbers, underscores, hyphens, or dots (e.g., "user.name").';
                isValid = false;
                continue;
            }

            // Validate claim path against claims_supported
            if (!this.isClaimPathInSupported(path)) {
                claim.error = `Claim path '${path}' is not in the IdP's supported claims list`;
                isValid = false;
                continue;
            }
        }

        // Check for duplicate paths
        const duplicateIndex = this.findDuplicateClaimIndex();
        if (duplicateIndex !== -1) {
            this.claims[duplicateIndex].error = 'Duplicate claim path';
            isValid = false;
        }

        return isValid;
    }

    ngOnDestroy(): void {
        if (this.valueChangesSub) {
            this.valueChangesSub.unsubscribe();
        }
    }

    initIdp(): FederatedIdp {
        return {
            id: undefined,
            name: '',
            description: '',
            issuer: '',
            supported_algorithms: [],
            claims_supported: [],
            offline_validation: false,
            openid_config_url: '',
            jwks_uri: '',
            jwks_keys: {},
            project_id: undefined,
        };
    }

    open(): void {
        this.createEditIdpOpened = true;
    }

    close(): void {
        this.createEditIdpOpened = false;
    }

    reset(): void {
        this.testOngoing = false;
        this.onGoing = false;

        if (
            this.targetForm &&
            this.targetForm.controls &&
            this.targetForm.controls.targetName
        ) {
            this.targetForm.controls.targetName.reset();
        }

        this.target = this.initIdp();
        this.initVal = this.initIdp();
        this.formValues = null;
        this.idpId = '';
        this.inlineAlert.close();
    }

    openCreateEditTarget(editable: boolean, targetId?: number | string) {
        this.editable = editable;
        this.reset();

        if (targetId) {
            this.idpId = targetId;
            this.translateService
                .get('FEDERATED_IDPS.TITLE_EDIT')
                .subscribe(res => (this.modalTitle = res));
            this.idpService.GetFederatedIdp({ id: Number(targetId) }).subscribe(
                target => {
                    this.target = target;
                    this.initVal = clone(target);

                    // Populate jwksKeys string from the target's jwks_keys object for UI display
                    if (
                        target.jwks_keys &&
                        Object.keys(target.jwks_keys).length > 0
                    ) {
                        this.jwksKeys = JSON.stringify(
                            target.jwks_keys,
                            null,
                            2
                        );
                    }

                    // Populate claimsSupported string from the target's claims_supported array
                    if (
                        target.claims_supported &&
                        target.claims_supported.length > 0
                    ) {
                        this.claimsSupported =
                            target.claims_supported.join(', ');
                    }

                    // Reconstruct openIDConfigJSON from stored fields for offline_validation IDPs
                    if (target.offline_validation && target.issuer) {
                        const reconstructedConfig: any = {
                            issuer: target.issuer,
                        };
                        if (target.jwks_uri) {
                            reconstructedConfig.jwks_uri = target.jwks_uri;
                        }
                        if (
                            target.claims_supported &&
                            target.claims_supported.length > 0
                        ) {
                            reconstructedConfig.claims_supported =
                                target.claims_supported;
                        }
                        if (
                            target.supported_algorithms &&
                            target.supported_algorithms.length > 0
                        ) {
                            reconstructedConfig.id_token_signing_alg_values_supported =
                                target.supported_algorithms;
                        }
                        this.openIDConfigJSON = JSON.stringify(
                            reconstructedConfig,
                            null,
                            2
                        );
                    }

                    this.idpService.ListClaimRules({ id: target.id }).subscribe(
                        claimRules => {
                            // Filter to only IDP-level claims (robot_id = 0 or null)
                            // Claims with robot_id > 0 are robot-specific and should not be shown here
                            const claims = claimRules
                                .filter(c => !c.robot_id || c.robot_id === 0)
                                .map(claimRule => {
                                    return {
                                        path: claimRule.claim_path,
                                        value: claimRule.value,
                                    };
                                });
                            this.claims = claims;
                            this.initClaims = clone(claims);
                        },
                        error => this.errorHandler.error(error)
                    );
                    this.open();
                    // this.editDisabled = true;
                },
                error => this.errorHandler.error(error)
            );
        } else {
            // this.urlDisabled = false;
            this.idpId = '';
            this.translateService
                .get('FEDERATED_IDPS.TITLE_ADD')
                .subscribe(res => (this.modalTitle = res));
            this.open();
            // this.editDisabled = false;
        }
    }

    onSubmit() {
        // added: safe optional chaining + fallback empty string
        this.target.name = this.target.name?.trim() ?? '';
        this.target.description = this.target.description?.trim() ?? '';
        this.target.issuer = this.target.issuer?.trim() ?? '';
        this.target.jwks_uri = this.target.jwks_uri?.trim() ?? '';
        this.target.openid_config_url =
            this.target.openid_config_url?.trim() ?? '';

        if (this.idpId) {
            this.updateIdp();
        } else {
            console.log('🚀 Create IDP');
            this.addIdp();
        }
    }

    // parse and get jwks_keys
    parseJwksKeys(): boolean {
        try {
            // 🆕 Empty check
            if (!this.jwksKeys || this.jwksKeys.trim() === '') {
                return false;
            }

            // 🆕 Safe JSON parsing
            const parsed = JSON.parse(this.jwksKeys);

            // 🆕 Validate JWKS structure
            if (!parsed.keys || !Array.isArray(parsed.keys)) {
                this.inlineAlert.showInlineError(
                    "Invalid JWKS: 'keys' array missing" // <-- replaced jwksError
                );
                return false;
            }

            // ✔️ Valid → assign it
            this.target.jwks_keys = parsed;

            return true; // <-- success
        } catch (err: any) {
            // 🆕 Use inline alert for failures
            this.inlineAlert.showInlineError(
                'Invalid JWKS JSON format. Please check the structure.' // <-- replaced jwksError
            );

            // ✔️ Prevent UI crash
            this.target.jwks_keys = null;

            return false; // <-- failure
        }
    }

    /**
     * Read-only validation for JWKS keys - does NOT modify state.
     * Used by isValid getter to avoid infinite loops.
     */
    isJwksKeysValid(): boolean {
        if (!this.jwksKeys || this.jwksKeys.trim() === '') {
            return false;
        }
        try {
            const parsed = JSON.parse(this.jwksKeys);
            return parsed.keys && Array.isArray(parsed.keys);
        } catch {
            return false;
        }
    }

    /**
     * Read-only validation for OpenID Config JSON - does NOT modify state.
     * Used by isValid getter to avoid infinite loops.
     */
    isOpenIDConfigValid(): boolean {
        if (!this.openIDConfigJSON || this.openIDConfigJSON.trim() === '') {
            return false;
        }
        try {
            const configObject = JSON.parse(this.openIDConfigJSON);
            return !!configObject.issuer;
        } catch {
            return false;
        }
    }

    addIdp() {
        if (this.onGoing) return;

        this.onGoing = true;
        this.okButtonState = ClrLoadingState.LOADING;
        console.log('this.target:', this.target);

        if (!this.validateRequiredClaims(this.claims)) {
            this.onGoing = false;
            this.okButtonState = ClrLoadingState.DEFAULT;
            return;
        }

        // parse and get jwks_keys
        if (!this.parseJwksKeys()) {
            this.onGoing = false;
            this.okButtonState = ClrLoadingState.DEFAULT;
            return;
        }

        this.idpService.CreateFederatedIdp({ idp: this.target }).subscribe(
            response => {
                console.log('create fed idp response:', response);
                console.log('this.claims:', this.claims);
                // assemble claim rules
                const assembledClaimRules = this.assembleClaimRules(
                    this.claims,
                    response.id
                );
                console.log('assembleClaimRules:', assembledClaimRules);

                const completeCreate = () => {
                    this.translateService
                        .get('FEDERATED_IDPS.CREATED_SUCCESS')
                        .subscribe(res => this.errorHandler.info(res));
                    this.reload.emit(true);
                    this.onGoing = false;
                    this.okButtonState = ClrLoadingState.SUCCESS;
                    this.close();
                };

                if (this.claims.length > 0) {
                    this.idpService
                        .CreateClaimRules({
                            id: response.id,
                            claims: { rules: assembledClaimRules },
                        })
                        .subscribe(
                            response => {
                                console.log(
                                    'create claim rules response:',
                                    response
                                );
                                completeCreate();
                            },
                            error => {
                                console.log('create claim rules error:', error);
                                this.onGoing = false;
                                this.okButtonState = ClrLoadingState.ERROR;
                                this.inlineAlert.showInlineError(error);
                            }
                        );
                    return;
                }
                completeCreate();
            },
            error => {
                this.onGoing = false;
                this.okButtonState = ClrLoadingState.ERROR;
                if (error.status === 409) {
                    this.inlineAlert.showInlineError(
                        'Federated IDP already exists with same name or issuer'
                    );
                } else {
                    this.inlineAlert.showInlineError(error);
                }
            }
        );
    }

    updateIdp() {
        if (this.onGoing || !this.target.id) return;
        if (!this.validateRequiredClaims(this.claims)) {
            this.onGoing = false;
            this.okButtonState = ClrLoadingState.DEFAULT;
            return;
        }

        const changes = this.getChanges();
        const claimsChanges = this.getClaimsChanges();
        if (isEmptyObject(changes) && isEmptyObject(claimsChanges)) return;

        // Build update payload with only allowed fields based on validation mode
        // Online mode: only description is editable
        // Offline mode: description AND jwks_keys are editable
        const updatedIdp: FederatedIdpUpdate = {};
        if (changes.description !== undefined) {
            updatedIdp.description = changes.description;
        }
        if (this.target.offline_validation && changes.jwks_keys !== undefined) {
            updatedIdp.jwks_keys = changes.jwks_keys;
        }

        const claimAddPayload = this.assembleClaimsToAddRules(
            claimsChanges.claimsToAdd,
            this.target.id
        );
        const claimDeletePayload = this.assembleClaimsToDeleteRules(
            claimsChanges.claimsToDelete,
            this.target.id
        );

        console.log('this.target:', this.target);
        console.log('this.changes:', changes);
        console.log('updatedIdp:', updatedIdp);

        this.onGoing = true;
        this.okButtonState = ClrLoadingState.LOADING;

        this.idpService
            .UpdateFederatedIdp({ idp: updatedIdp, id: this.target.id })
            .subscribe(
                () => {
                    this.translateService
                        .get('FEDERATED_IDPS.UPDATED_SUCCESS')
                        .subscribe(res => this.errorHandler.info(res));
                    const assembledClaimRules = this.assembleClaimRules(
                        this.claims,
                        this.target.id
                    );
                    console.log('assembleClaimRules:', assembledClaimRules);

                    if (claimDeletePayload.length > 0) {
                        this.idpService
                            .DeleteClaimRules({
                                id: this.target.id,
                                claims: { rules: claimDeletePayload },
                            })
                            .subscribe(
                                response => {
                                    console.log(
                                        'create claim rules response:',
                                        response
                                    );
                                },
                                error => {
                                    console.log(
                                        'create claim rules error:',
                                        error
                                    );
                                    this.onGoing = false;
                                    this.okButtonState = ClrLoadingState.ERROR;
                                    this.inlineAlert.showInlineError(error);
                                }
                            );
                    }

                    if (claimAddPayload.length > 0) {
                        this.idpService
                            .CreateClaimRules({
                                id: this.target.id,
                                claims: { rules: claimAddPayload },
                            })
                            .subscribe(
                                response => {
                                    console.log(
                                        'create claim rules response:',
                                        response
                                    );
                                },
                                error => {
                                    console.log(
                                        'create claim rules error:',
                                        error
                                    );
                                    this.onGoing = false;
                                    this.okButtonState = ClrLoadingState.ERROR;
                                    this.inlineAlert.showInlineError(error);
                                }
                            );
                    }
                },
                error => {
                    this.inlineAlert.showInlineError(error);
                    this.onGoing = false;
                    this.okButtonState = ClrLoadingState.ERROR;
                }
            );
        this.reload.emit(true);
        this.close();
        this.onGoing = false;
        this.okButtonState = ClrLoadingState.SUCCESS;
    }

    onCancel() {
        const claimsChanges = this.getClaimsChanges();
        const changes = this.getChanges();

        if (
            !isEmptyObject(changes) ||
            claimsChanges.claimsToAdd.length > 0 ||
            claimsChanges.claimsToDelete.length > 0
        ) {
            this.inlineAlert.showInlineConfirmation({
                message: 'ALERT.FORM_CHANGE_CONFIRMATION',
            });
        } else {
            this.close();
            if (this.targetForm) {
                this.targetForm.reset();
            }
            this.reset();
            this.reload.emit(true);
            this.claimsSupported = '';
            this.claims = [
                {
                    path: 'iss',
                    value: '',
                },
            ];
            this.jwksKeys = '';
        }
    }

    validateRequiredClaims(claims: Claim[]): boolean {
        // Check if claims is valid array
        if (!claims || !Array.isArray(claims)) {
            console.error('Invalid claims array provided.');
            return false;
        }

        // Trim all claims before validation
        claims.forEach(c => this.trimClaim(c));

        // Run full validation (sets error messages on claims)
        if (!this.validateAllClaims()) {
            // Find the first error to display in inline alert
            const firstError = claims.find(c => c.error);
            if (firstError) {
                this.inlineAlert.showInlineError(firstError.error);
            }
            return false;
        }

        // Extract all claim paths (trimmed and lowercased)
        const paths = claims
            .filter(c => c.path && c.path.trim())
            .map(c => c.path.trim().toLowerCase());

        // Required claim keys
        const requiredKeys = ['iss'];

        // Find missing ones
        const missing = requiredKeys.filter(key => !paths.includes(key));

        if (missing.length > 0) {
            console.error(
                `Missing required claim path(s): ${missing.join(', ')}`
            );
            this.inlineAlert.showInlineError(
                `Missing required claim path(s): ${missing.join(', ')}`
            );
            return false;
        }

        // Check for duplicate paths
        if (this.hasDuplicateClaimPaths()) {
            this.inlineAlert.showInlineError(
                'Duplicate claim paths found. Each claim path must be unique.'
            );
            return false;
        }

        // Check that all claims have non-empty values
        for (const claim of claims) {
            const path = (claim.path || '').trim();
            const value = (claim.value || '').trim();

            // Skip empty claims
            if (!path && !value) {
                continue;
            }

            if (path && !value) {
                this.inlineAlert.showInlineError(
                    `Claim "${path}" has an empty value. All claims must have values.`
                );
                return false;
            }
        }

        // Everything valid
        return true;
    }

    assembleClaimsToAddRules(claimsToAdd: Claim[], id: number) {
        if (!claimsToAdd || !Array.isArray(claimsToAdd)) {
            console.error("Input 'claimsToAdd' is not a valid array.");
            return [];
        }
        if (id === 0 || id === undefined) {
            console.error("Input is missing an 'id' property.");
            return [];
        }

        // Builds rules for new claims to insert (trimmed values)
        return claimsToAdd
            .filter(claim => claim.path?.trim() && claim.value?.trim())
            .map(claim => ({
                claim_path: claim.path.trim(),
                value: claim.value.trim(),
                identity_provider_id: id,
                action: 'add',
            }));
    }

    assembleClaimsToDeleteRules(claimsToDelete: Claim[], id: number) {
        if (!claimsToDelete || !Array.isArray(claimsToDelete)) {
            console.error("Input 'claimsToDelete' is not a valid array.");
            return [];
        }
        if (id === 0 || id === undefined) {
            console.error("Input is missing an 'id' property.");
            return [];
        }

        // Builds rules for claims to remove (trimmed values)
        return claimsToDelete
            .filter(claim => claim.path?.trim() && claim.value?.trim())
            .map(claim => ({
                claim_path: claim.path.trim(),
                value: claim.value.trim(),
                identity_provider_id: id,
                action: 'delete',
            }));
    }

    assembleClaimRules(claims: Claim[], id: number) {
        if (!claims || !Array.isArray(claims)) {
            console.error("Input 'claims' is not a valid array.");
            return [];
        }
        if (id === 0 || id === undefined) {
            console.error("Input is missing an 'id' property.");
            return [];
        }

        // Filter out empty claims and trim values before sending to API
        return claims
            .filter(claim => claim.path?.trim() && claim.value?.trim())
            .map(claim => ({
                claim_path: claim.path.trim(),
                value: claim.value.trim(),
                identity_provider_id: id,
            }));
    }

    confirmCancel(confirmed: boolean) {
        this.inlineAlert.close();
        this.claimsSupported = '';
        this.claims = [
            {
                path: 'iss',
                value: '',
            },
        ];
        this.jwksKeys = '';
        this.reset();
        if (this.targetForm) {
            this.targetForm.reset();
        }
        this.reload.emit(true);
        this.close();
    }

    ngAfterViewChecked(): void {
        if (this.targetForm !== this.currentForm) {
            this.targetForm = this.currentForm;
            if (this.targetForm) {
                this.valueChangesSub = this.targetForm.valueChanges.subscribe(
                    (data: any) => {
                        if (!compareValue(this.formValues, data)) {
                            this.formValues = data;
                        }
                    }
                );
            }
        }
    }

    getClaimsChanges(): {
        claimsToAdd: Claim[];
        claimsToDelete: Claim[];
    } {
        const claimsToAdd: Claim[] = [];
        const claimsToDelete: Claim[] = [];

        // Return early if either array is empty
        if (!this.claims?.length || !this.initClaims?.length) {
            // Handle case where claims were added to an initially empty list
            if (this.claims?.length && !this.initClaims?.length) {
                return {
                    claimsToAdd: this.claims.filter(
                        c => c.path?.trim() && c.value?.trim()
                    ),
                    claimsToDelete: [],
                };
            }
            return { claimsToAdd, claimsToDelete };
        }

        // Convert both arrays into Map for O(1) lookup by `path` (trimmed)
        const currentMap = new Map(
            this.claims
                .filter(c => c.path?.trim())
                .map(c => [c.path.trim(), c.value?.trim() || ''])
        );
        const initMap = new Map(
            this.initClaims
                .filter(c => c.path?.trim())
                .map(c => [c.path.trim(), c.value?.trim() || ''])
        );

        // Iterate over initial claims to detect deletions or modifications
        Array.from(initMap.entries()).forEach(([path, oldValue]) => {
            const newValue = currentMap.get(path);

            if (newValue === undefined) {
                // Claim removed → delete
                claimsToDelete.push({ path, value: oldValue });
            } else if (!compareValue(oldValue, newValue)) {
                // Modified → delete old + add new
                claimsToDelete.push({ path, value: oldValue });
                claimsToAdd.push({ path, value: newValue });
            }
        });

        // Detect newly added claims
        Array.from(currentMap.entries()).forEach(([path, newValue]) => {
            if (!initMap.has(path)) {
                // New claim added
                claimsToAdd.push({ path, value: newValue });
            }
        });

        return { claimsToAdd, claimsToDelete };
    }

    getChanges(): { [key: string]: any | any[] } {
        const changes: { [key: string]: any | any[] } = {};
        if (!this.target || !this.initVal) return changes;

        // Get all unique keys WITHOUT spreading arrays
        const keys = new Set([
            ...Object.keys(this.target),
            ...Object.keys(this.initVal),
        ]);

        Array.from(keys).forEach(prop => {
            const original = this.initVal[prop];
            const current = this.target[prop];

            // If either side is an array, do not treat like object
            if (Array.isArray(original) || Array.isArray(current)) {
                // deep compare arrays safely
                if (!compareValue(original, current)) {
                    changes[prop] = current; // return the whole array safely
                }
                return; // skip object logic for arrays
            }

            // non-object or primitive
            if (typeof original !== 'object' || original === null) {
                if (!compareValue(original, current)) {
                    changes[prop] =
                        typeof original === 'string'
                            ? ('' + current).trim()
                            : current;
                }
                return;
            }

            // Handle objects safely without spreading arrays accidentally
            const subKeys = new Set([
                ...Object.keys(original || {}),
                ...Object.keys(current || {}),
            ]);

            Array.from(subKeys).forEach(subProp => {
                if (!compareValue(original[subProp], current[subProp])) {
                    changes[subProp] =
                        typeof original[subProp] === 'string'
                            ? ('' + current[subProp]).trim()
                            : current[subProp];
                }
            });
        });

        return changes;
    }
}
