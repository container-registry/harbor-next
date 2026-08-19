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
import { Component, OnInit, OnDestroy, ViewChild } from '@angular/core';
import {
    Subscription,
    Observable,
    forkJoin,
    throwError as observableThrowError,
} from 'rxjs';
import { TranslateService } from '@ngx-translate/core';
import {
    Comparator,
    UserPermissionService,
    USERSTATICPERMISSION,
} from '../../../shared/services';
import { ErrorHandler } from '../../../shared/units/error-handler';
import { map, catchError, finalize } from 'rxjs/operators';
import { ConfirmationDialogComponent } from '../../../shared/components/confirmation-dialog';
import {
    ConfirmationTargets,
    ConfirmationState,
    ConfirmationButtons,
    PAGE_SIZE_OPTIONS,
} from '../../../shared/entities/shared.const';
import {
    CustomComparator,
    getPageSizeFromLocalStorage,
    getSortingString,
    PageSizeMapKeys,
    setPageSizeToLocalStorage,
} from '../../../shared/units/utils';
import {
    operateChanges,
    OperateInfo,
    OperationState,
} from '../../../shared/components/operation/operate';
import { OperationService } from '../../../shared/components/operation/operation.service';
import { errorHandler } from '../../../shared/units/shared.utils';
import { ConfirmationMessage } from '../../global-confirmation-dialog/confirmation-message';
import { ConfirmationAcknowledgement } from '../../global-confirmation-dialog/confirmation-state-message';
import { ClrDatagridStateInterface } from '@clr/angular';
import { FederatedIdp } from 'ng-swagger-gen/models';
import { FederatedIdpService } from 'ng-swagger-gen/services';
import { AddIdpComponent } from './add-idp/add-idp.component';
import { ActivatedRoute } from '@angular/router';

@Component({
    selector: 'federated-idp',
    templateUrl: './federated-idp.component.html',
    styleUrls: ['./federated-idp.component.scss'],
})
export class FederatedIdpComponent implements OnInit, OnDestroy {
    clrPageSizeOptions: number[] = PAGE_SIZE_OPTIONS;
    @ViewChild(AddIdpComponent)
    addIdpComponent: AddIdpComponent;

    @ViewChild('confirmationDialog')
    confirmationDialogComponent: ConfirmationDialogComponent;

    targets: FederatedIdp[];
    target: FederatedIdp;

    targetName: string;
    subscription: Subscription;

    loading: boolean = true;

    creationTimeComparator: Comparator<FederatedIdp> =
        new CustomComparator<FederatedIdp>('creation_time', 'date');

    timerHandler: any;
    selectedRow: FederatedIdp[] = [];

    // Project context
    projectId: number;
    projectName: string;

    // Permission flags
    hasCreatePermission: boolean = false;
    hasUpdatePermission: boolean = false;
    hasDeletePermission: boolean = false;
    hasReadPermission: boolean = false;

    get initIdp(): FederatedIdp {
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
            project_id: this.projectId,
            creation_time: '',
            update_time: '',
        };
    }

    pageSize: number = getPageSizeFromLocalStorage(
        PageSizeMapKeys.SYSTEM_ENDPOINT_COMPONENT
    );
    page: number = 1;
    total: number = 0;
    constructor(
        private idpService: FederatedIdpService,
        private errorHandlerEntity: ErrorHandler,
        private translateService: TranslateService,
        private operationService: OperationService,
        private route: ActivatedRoute,
        private userPermissionService: UserPermissionService
    ) {}

    ngOnInit(): void {
        this.targetName = '';
        // Get project ID from route (same pattern as robot-account.component.ts)
        this.projectId = +this.route.snapshot.parent.parent.params['id'];
        const resolverData = this.route.snapshot.parent.parent.data;
        if (resolverData) {
            const project = resolverData['projectResolver'];
            if (project) {
                this.projectName = project.name;
            }
        }
        this.getPermissionsList();
    }

    getPermissionsList(): void {
        const permissionsList: Observable<boolean>[] = [];
        permissionsList.push(
            this.userPermissionService.getPermission(
                this.projectId,
                USERSTATICPERMISSION.FEDERATED_IDP.KEY,
                USERSTATICPERMISSION.FEDERATED_IDP.VALUE.CREATE
            )
        );
        permissionsList.push(
            this.userPermissionService.getPermission(
                this.projectId,
                USERSTATICPERMISSION.FEDERATED_IDP.KEY,
                USERSTATICPERMISSION.FEDERATED_IDP.VALUE.UPDATE
            )
        );
        permissionsList.push(
            this.userPermissionService.getPermission(
                this.projectId,
                USERSTATICPERMISSION.FEDERATED_IDP.KEY,
                USERSTATICPERMISSION.FEDERATED_IDP.VALUE.DELETE
            )
        );
        permissionsList.push(
            this.userPermissionService.getPermission(
                this.projectId,
                USERSTATICPERMISSION.FEDERATED_IDP.KEY,
                USERSTATICPERMISSION.FEDERATED_IDP.VALUE.READ
            )
        );

        forkJoin(permissionsList).subscribe({
            next: rules => {
                [
                    this.hasCreatePermission,
                    this.hasUpdatePermission,
                    this.hasDeletePermission,
                    this.hasReadPermission,
                ] = rules;
            },
            error: error => this.errorHandlerEntity.error(error),
        });
    }

    ngOnDestroy(): void {
        if (this.subscription) {
            this.subscription.unsubscribe();
        }
    }

    retrieve(state?: ClrDatagridStateInterface): void {
        this.selectedRow = [];
        // Build query for project-level IdPs
        let q: string = `Level=project,ProjectID=${this.projectId}`;
        if (state && state.filters && state.filters.length) {
            this.targetName = '';
            q += `,${state.filters[0].property}=~${state.filters[0].value}`;
        } else if (this.targetName) {
            q += `,name=~${this.targetName}`;
        }

        if (state && state.page) {
            this.pageSize = state.page.size;
            setPageSizeToLocalStorage(
                PageSizeMapKeys.SYSTEM_ENDPOINT_COMPONENT,
                this.pageSize
            );
        }
        let sort: string;
        if (state && state.sort && state.sort.by) {
            sort = getSortingString(state);
        } else {
            // sort by creation_time desc by default
            sort = `-creation_time`;
        }
        this.loading = true;
        this.idpService
            .ListFederatedIdps({
                q: encodeURIComponent(q),
                pageSize: this.pageSize,
                page: this.page,
                sort: sort,
            })
            .pipe(
                finalize(() => {
                    this.loading = false;
                })
            )
            .subscribe({
                next: response => {
                    this.targets = response || [];
                },
                error: error => {
                    this.errorHandlerEntity.error(error);
                },
            });
    }

    doSearchTargets(targetName: string) {
        this.targetName = targetName;
        this.page = 1;
        this.total = 0;
        this.selectedRow = [];
        this.retrieve();
    }

    refreshTargets() {
        this.targetName = '';
        this.page = 1;
        this.total = 0;
        this.selectedRow = [];
        this.retrieve();
    }

    openModal() {
        if (this.addIdpComponent) {
            this.addIdpComponent.projectId = this.projectId;
            this.addIdpComponent.openCreateEditTarget(true);
        }
        this.target = this.initIdp;
    }

    editTargets(targets: FederatedIdp[]) {
        if (targets && targets.length === 1) {
            const target = targets[0];
            if (!target.id) {
                return;
            }
            if (this.addIdpComponent) {
                this.addIdpComponent.projectId = this.projectId;
                this.addIdpComponent.openCreateEditTarget(true, target.id);
            }
        }
    }

    deleteTargets(targets: FederatedIdp[]) {
        if (targets && targets.length) {
            const targetNames: string[] = [];
            targets.forEach(target => {
                targetNames.push(target.name);
            });
            const deletionMessage = new ConfirmationMessage(
                'FEDERATED_IDPS.DELETION_TITLE',
                'FEDERATED_IDPS.DELETION_SUMMARY',
                targetNames.join(', ') || '',
                targets,
                ConfirmationTargets.TARGET,
                ConfirmationButtons.DELETE_CANCEL
            );
            this.confirmationDialogComponent.open(deletionMessage);
        }
    }

    confirmDeletion(message: ConfirmationAcknowledgement) {
        if (
            message &&
            message.source === ConfirmationTargets.TARGET &&
            message.state === ConfirmationState.CONFIRMED
        ) {
            const targetLists: FederatedIdp[] = message.data;
            if (targetLists && targetLists.length) {
                const observableLists: Observable<any>[] = [];
                targetLists.forEach(target => {
                    observableLists.push(this.delOperate(target));
                });
                forkJoin(observableLists)
                    .pipe(
                        finalize(() => {
                            this.refreshTargets();
                        })
                    )
                    .subscribe({
                        next: () => {},
                        error: error => {
                            this.errorHandlerEntity.error(error);
                        },
                    });
            }
        }
    }

    delOperate(target: FederatedIdp): Observable<any> {
        // init operation info
        const operMessage = new OperateInfo();
        operMessage.name = 'OPERATION.DELETE_IDP';
        operMessage.data.id = target.id;
        operMessage.state = OperationState.progressing;
        operMessage.data.name = target.name;
        this.operationService.publishInfo(operMessage);
        return this.idpService
            .DeleteFederatedIdp({
                id: target.id,
            })
            .pipe(
                map(() => {
                    this.translateService
                        .get('BATCH.DELETED_SUCCESS')
                        .subscribe(() => {
                            operateChanges(operMessage, OperationState.success);
                        });
                }),
                catchError(error => {
                    const errorMessage = errorHandler(error);
                    this.translateService
                        .get(errorMessage)
                        .subscribe(res =>
                            operateChanges(
                                operMessage,
                                OperationState.failure,
                                res
                            )
                        );
                    return observableThrowError(() => error);
                })
            );
    }

    // give supported claims as comma separated string
    getSupportedClaims(claims: string[]): string {
        if (!claims || claims.length === 0) {
            return '';
        }
        const fullText = claims.join(', ');
        const maxLength = 18;
        return fullText.length > maxLength
            ? fullText.slice(0, maxLength) + '…'
            : fullText;
    }

    // give supported algorithms as comma separated string
    getSupportedAlgorithms(algorithms: string[]): string {
        if (!algorithms || algorithms.length === 0) {
            return '';
        }
        return algorithms.join(', ');
    }
}
