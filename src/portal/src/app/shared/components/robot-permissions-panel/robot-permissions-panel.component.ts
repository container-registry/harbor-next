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
    DoCheck,
    ElementRef,
    EventEmitter,
    HostBinding,
    Input,
    OnChanges,
    OnInit,
    Output,
    SimpleChanges,
    ViewChild,
} from '@angular/core';
import {
    convertKey,
    hasPermission,
    isCandidate,
    SUB_ROW_ACTIONS,
} from '../../../base/left-side-nav/system-robot-accounts/system-robot-util';
import { Access } from '../../../../../ng-swagger-gen/models/access';
import { Permission } from '../../../../../ng-swagger-gen/models/permission';

@Component({
    selector: 'robot-permissions-panel',
    templateUrl: './robot-permissions-panel.component.html',
    styleUrls: ['./robot-permissions-panel.component.scss'],
})
export class RobotPermissionsPanelComponent implements OnChanges, DoCheck {
    modalOpen: boolean = false;

    @Input()
    mode: PermissionSelectPanelModes = PermissionSelectPanelModes.NORMAL;

    // In a horizontal form the panel is a flex item; let the table span the row.
    @HostBinding('class.fill')
    get fill(): boolean {
        return this.mode === PermissionSelectPanelModes.NORMAL;
    }

    @Input()
    dropdownPosition: string = 'bottom-left';

    @Input()
    usedInDatagrid: boolean = false;

    @Input()
    candidatePermissions: Permission[] = [];

    candidateActions: string[] = [];
    candidateResources: string[] = [];
    // Resources with sub-rows are disclosure rows; collapsed ones hide their sub-rows.
    collapsed: { [resource: string]: boolean } = {};

    @Input()
    permissionsModel!: Access[];
    @Output()
    permissionsModelChange = new EventEmitter<Access[]>();

    @ViewChild('dropdownMenu')
    dropdownMenu: any;

    @ViewChild('dropdown')
    dropdown: ElementRef;

    // to avoid ng check error, getTransform() should always return 'unset' before dropdownMenu appears
    dropdownMenuAppeared: boolean = false;
    getTransform(): string {
        if (
            this.dropdownMenuAppeared &&
            this.dropdownMenu?.el &&
            this.dropdown
        ) {
            const width = this.dropdownMenu.el.nativeElement.offsetWidth;
            const height = this.dropdownMenu.el.nativeElement.offsetHeight;
            const bcr = this.dropdown.nativeElement.getBoundingClientRect();
            return `translateX(${bcr.x - width}px) translateY(${
                bcr.y - height / 2
            }px)`;
        }
        return 'unset';
    }

    ngOnChanges(changes: SimpleChanges) {
        if (changes && changes['candidatePermissions']) {
            this.initCandidates();
        }
    }
    ngDoCheck() {
        this.dropdownMenuAppeared = !!this.dropdownMenu;
    }

    initCandidates() {
        this.candidateActions = [];
        this.candidateResources = [];
        this.candidatePermissions?.forEach(item => {
            if (this.candidateResources.indexOf(item?.resource) === -1) {
                this.candidateResources.push(item?.resource);
            }
            if (
                this.candidateActions.indexOf(item?.action) === -1 &&
                SUB_ROW_ACTIONS.indexOf(item?.action) === -1
            ) {
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
        return (this.candidatePermissions || []).every(item =>
            hasPermission(this.permissionsModel, item)
        );
    }

    selectAllOrUnselectAll() {
        // A robot may hold permissions this panel does not offer, such as a
        // grant only a system admin can give; leave those as they are.
        const kept = (this.permissionsModel || []).filter(
            item => !this.isCandidate(item.resource, item.action)
        );
        if (this.isAllSelected()) {
            this.permissionsModel = kept;
        } else {
            this.permissionsModel = kept.concat(
                (this.candidatePermissions || []).map(item => ({
                    resource: item.resource,
                    action: item.action,
                }))
            );
            this.collapsed = {};
        }
        this.permissionsModelChange.emit(this.permissionsModel);
    }

    hasSubRows(resource: string): boolean {
        return this.subRowsOf(resource).length > 0;
    }

    isExpanded(resource: string): boolean {
        return !this.collapsed[resource];
    }

    toggleExpanded(resource: string) {
        this.collapsed[resource] = !this.collapsed[resource];
    }

    // Actions that are not grid columns and render as a sub-row of their resource.
    subRowsOf(resource: string): string[] {
        return SUB_ROW_ACTIONS.filter(action =>
            this.isCandidate(resource, action)
        );
    }

    // The column that carries a sub-row's checkbox: Create, or the first column.
    subRowColumn(): string {
        return this.candidateActions.indexOf('create') !== -1
            ? 'create'
            : this.candidateActions[0] || '';
    }

    // One cell per action column; a single cell when there is no column, so the
    // checkbox still shows for a robot that holds only sub-row permissions.
    subRowCells(): string[] {
        return this.candidateActions.length ? this.candidateActions : [''];
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
