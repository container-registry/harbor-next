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
    OnDestroy,
    ViewChild,
    AfterViewInit,
} from '@angular/core';
import {
    Subscription,
    Observable,
    forkJoin,
    throwError as observableThrowError,
} from 'rxjs';
import { TranslateService } from '@ngx-translate/core';
import { Comparator } from '../../../shared/services';
import { ErrorHandler } from '../../../shared/units/error-handler';
import { map, catchError, finalize } from 'rxjs/operators';
import { ConfirmationDialogComponent } from '../../../shared/components/confirmation-dialog';
import {
    ConfirmationTargets,
    ConfirmationState,
    ConfirmationButtons,
    PAGE_SIZE_OPTIONS,
} from '../../../shared/entities/shared.const';
// TODO: search for /endpoint and replace everything with idp and fix the errors
// import { CreateEditEndpointComponent } from './create-edit-endpoint/create-edit-endpoint.component';
//
// uu
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
import { CreateEditIdpComponent } from './create-edit-idp/create-edit-idp.component';

@Component({
    selector: 'federated-idps',
    templateUrl: './idp.component.html',
    styleUrls: ['./idp.component.scss'],
})
export class IdpComponent implements OnInit, OnDestroy {
    clrPageSizeOptions: number[] = PAGE_SIZE_OPTIONS;
    // TODO: add create edit idp component
    @ViewChild(CreateEditIdpComponent)
    CreateEditIdpComponent: CreateEditIdpComponent;

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

    // TODO:remove the registry and create idp
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
            project_id: undefined,
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
        private operationService: OperationService
    ) {}

    ngOnInit(): void {
        this.targetName = '';
    }

    ngOnDestroy(): void {
        if (this.subscription) {
            this.subscription.unsubscribe();
        }
    }
    retrieve(state?: ClrDatagridStateInterface): void {
        this.selectedRow = [];
        let q: string = '';
        if (state && state.filters && state.filters.length) {
            this.targetName = '';
            q = encodeURIComponent(
                `${state.filters[0].property}=~${state.filters[0].value}`
            );
        } else if (this.targetName) {
            q = `name=~${this.targetName}`;
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
                q: q,
                pageSize: this.pageSize,
                page: this.page,
                sort: sort,
            })
            .pipe(
                finalize(() => {
                    this.loading = false;
                })
            )
            .subscribe(
                response => {
                    // console.log('response: ', response);
                    // Get total count
                    // if (response) {
                    //     let xHeader: string =
                    //         response.headers.get('X-Total-Count');
                    //     if (xHeader) {
                    //         this.total = parseInt(xHeader, 0);
                    //     }
                    // }
                    this.targets = response || [];
                },
                error => {
                    this.errorHandlerEntity.error(error);
                }
            );
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
        this.CreateEditIdpComponent.openCreateEditTarget(true);
        this.target = this.initIdp;
    }

    editTargets(targets: FederatedIdp[]) {
        if (targets && targets.length === 1) {
            let target = targets[0];
            let editable = true;
            if (!target.id) {
                return;
            }
            let id: number | string = target.id;
            this.CreateEditIdpComponent.openCreateEditTarget(editable, id);
        }
    }

    deleteTargets(targets: FederatedIdp[]) {
        if (targets && targets.length) {
            let targetNames: string[] = [];
            targets.forEach(target => {
                targetNames.push(target.name);
            });
            let deletionMessage = new ConfirmationMessage(
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
            let targetLists: FederatedIdp[] = message.data;
            if (targetLists && targetLists.length) {
                let observableLists: any[] = [];
                targetLists.forEach(target => {
                    observableLists.push(this.delOperate(target));
                });
                forkJoin(...observableLists)
                    .pipe(
                        finalize(() => {
                            this.refreshTargets();
                        })
                    )
                    .subscribe(
                        item => {},
                        error => {
                            this.errorHandlerEntity.error(error);
                        }
                    );
            }
        }
    }
    delOperate(target: FederatedIdp): Observable<any> {
        // init operation info
        let operMessage = new OperateInfo();
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
                map(response => {
                    this.translateService
                        .get('BATCH.DELETED_SUCCESS')
                        .subscribe(res => {
                            operateChanges(operMessage, OperationState.success);
                        });
                }),
                catchError(error => {
                    const message = errorHandler(error);
                    this.translateService
                        .get(message)
                        .subscribe(res =>
                            operateChanges(
                                operMessage,
                                OperationState.failure,
                                res
                            )
                        );
                    return observableThrowError(error);
                })
            );
    }

    // give supported claims as comma separated string
    getSupportedClaims(claims: string[]): string {
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
