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
    AfterViewInit,
    Component,
    EventEmitter,
    Input,
    OnChanges,
    OnInit,
    Output,
    SimpleChanges,
} from '@angular/core';
import {
    convertKey,
    hasPermission,
    isCandidate,
} from '../../../base/left-side-nav/system-robot-accounts/system-robot-util';
import { Access } from '../../../../../ng-swagger-gen/models/access';
import { Permission } from '../../../../../ng-swagger-gen/models/permission';

@Component({
    selector: 'robot-permissions-panel',
    templateUrl: './robot-permissions-panel.component.html',
    styleUrls: ['./robot-permissions-panel.component.scss'],
})
export class RobotPermissionsPanelComponent implements OnChanges {
    modalOpen: boolean = false;

    @Input()
    mode: PermissionSelectPanelModes = PermissionSelectPanelModes.NORMAL;

    @Input()
    dropdownPosition: string = 'bottom-left';

    @Input()
    usedInDatagrid: boolean = false;

    @Input()
    candidatePermissions: Permission[] = [];

    candidateActions: string[] = [];
    candidateResources: string[] = [];

    @Input()
    permissionsModel!: Access[];
    @Output()
    permissionsModelChange = new EventEmitter<Access[]>();

    ngOnChanges(changes: SimpleChanges) {
        if (changes && changes['candidatePermissions']) {
            this.initCandidates();
        }
    }

    initCandidates() {
        this.candidateActions = [];
        this.candidateResources = [];
        this.candidatePermissions?.forEach(item => {
            const resource = item?.resource ?? '';
            if (this.candidateResources.indexOf(resource) === -1) {
                this.candidateResources.push(resource);
            }
            if (this.candidateActions.indexOf(item?.action) === -1) {
                this.candidateActions.push(item?.action);
            }
        });
        this.candidateActions.sort();
        this.candidateResources.sort();
    }

    isCandidate(resource: string, action: string): boolean {
        return isCandidate(this.candidatePermissions, { resource, action });
    }

    getCheckBoxValue(resource: string, action: string): boolean {
        return hasPermission(this.permissionsModel, { resource, action });
    }

    setCheckBoxValue(resource: string, action: string, value: boolean) {
        if (value) {
            if (!this.permissionsModel) {
                this.permissionsModel = [];
            }
            this.permissionsModel.push({ resource, action });
        } else {
            this.permissionsModel = this.permissionsModel.filter(item => {
                return item.resource !== resource || item.action !== action;
            });
        }
        this.permissionsModelChange.emit(this.permissionsModel);
    }

    isAllSelected(): boolean {
        let flag: boolean = true;
        this.candidateActions.forEach(action => {
            this.candidateResources.forEach(resource => {
                if (
                    this.isCandidate(resource, action) &&
                    !hasPermission(this.permissionsModel, { resource, action })
                ) {
                    flag = false;
                }
            });
        });
        return flag;
    }

    selectAllOrUnselectAll() {
        if (this.isAllSelected()) {
            this.permissionsModel = [];
        } else {
            this.permissionsModel = [];
            this.candidateActions.forEach(action => {
                this.candidateResources.forEach(resource => {
                    if (this.isCandidate(resource, action)) {
                        this.permissionsModel.push({ resource, action });
                    }
                });
            });
        }
        this.permissionsModelChange.emit(this.permissionsModel);
    }

    convertKey(key: string): string {
        return convertKey(key);
    }
    protected readonly PermissionSelectPanelModes = PermissionSelectPanelModes;
}

export enum PermissionSelectPanelModes {
    DROPDOWN,
    MODAL,
    NORMAL,
}
