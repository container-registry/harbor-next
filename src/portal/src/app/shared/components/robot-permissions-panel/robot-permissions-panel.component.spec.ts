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
import { Component, ViewChild } from '@angular/core';
import { SharedTestingModule } from '../../shared.module';
import {
    PermissionSelectPanelModes,
    RobotPermissionsPanelComponent,
} from './robot-permissions-panel.component';
import { hasPermission } from '../../../base/left-side-nav/system-robot-accounts/system-robot-util';

describe('RobotPermissionsPanelComponent', () => {
    let component: TestHostComponent;
    let fixture: ComponentFixture<TestHostComponent>;
    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [SharedTestingModule],
            declarations: [TestHostComponent, RobotPermissionsPanelComponent],
        }).compileComponents();
    });

    beforeEach(() => {
        fixture = TestBed.createComponent(TestHostComponent);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should render right mode', async () => {
        component.robotPermissionsPanelComponent.modalOpen = true;
        fixture.detectChanges();
        await fixture.whenStable();
        const table = fixture.nativeElement.querySelector('table');
        expect(table).toBeTruthy();
        component.mode = PermissionSelectPanelModes.DROPDOWN;
        fixture.detectChanges();
        await fixture.whenStable();
        const clrDropdown = fixture.nativeElement.querySelector('clr-dropdown');
        expect(clrDropdown).toBeTruthy();
        component.mode = PermissionSelectPanelModes.MODAL;
        fixture.detectChanges();
        await fixture.whenStable();
        const modal = fixture.nativeElement.querySelector('clr-modal');
        expect(modal).toBeTruthy();
    });
});

describe('RobotPermissionsPanelComponent with member:grant-admin', () => {
    const pull = { resource: 'repository', action: 'pull' };
    const createMember = { resource: 'member', action: 'create' };
    const grantAdmin = { resource: 'member', action: 'grant-admin' };
    let fixture: ComponentFixture<RobotPermissionsPanelComponent>;
    let panel: RobotPermissionsPanelComponent;

    function render(candidates) {
        fixture = TestBed.createComponent(RobotPermissionsPanelComponent);
        panel = fixture.componentInstance;
        panel.mode = PermissionSelectPanelModes.NORMAL;
        panel.candidatePermissions = candidates;
        panel.initCandidates();
        panel.permissionsModel = [];
        fixture.detectChanges();
    }

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [SharedTestingModule],
            declarations: [RobotPermissionsPanelComponent],
        }).compileComponents();
        render([pull, createMember, grantAdmin]);
    });

    it('select all leaves member:grant-admin unticked', () => {
        panel.selectAllOrUnselectAll();
        expect(hasPermission(panel.permissionsModel, pull)).toBeTrue();
        expect(hasPermission(panel.permissionsModel, createMember)).toBeTrue();
        expect(hasPermission(panel.permissionsModel, grantAdmin)).toBeFalse();
    });

    it('counts everything but member:grant-admin as all selected', () => {
        panel.permissionsModel = [pull, createMember];
        expect(panel.isAllSelected()).toBeTrue();
    });

    it('select all keeps a member:grant-admin ticked on its own', () => {
        panel.setCheckBoxValue(grantAdmin.resource, grantAdmin.action, true);
        expect(panel.isAllSelected()).toBeFalse();
        panel.selectAllOrUnselectAll();
        expect(panel.permissionsModel.length).toBe(3);
        expect(hasPermission(panel.permissionsModel, grantAdmin)).toBeTrue();
    });

    it('warns what member:grant-admin allows when it is offered', () => {
        const hint = fixture.nativeElement.querySelector('.grant-admin-hint');
        expect(hint).toBeTruthy();
        expect(hint.textContent).toContain('ROBOT_ACCOUNT.GRANT_ADMIN_HINT');
    });

    it('shows no warning when member:grant-admin is not offered', () => {
        render([pull, createMember]);
        expect(
            fixture.nativeElement.querySelector('.grant-admin-hint')
        ).toBeNull();
    });
});

// mock a TestHostComponent for RobotPermissionsPanelComponent
@Component({
    template: `
        <ng-container *ngIf="mode === PermissionSelectPanelModes.MODAL">
            <robot-permissions-panel [mode]="mode">
                <div>modal</div>
            </robot-permissions-panel>
        </ng-container>
        <ng-container *ngIf="mode === PermissionSelectPanelModes.DROPDOWN">
            <robot-permissions-panel [mode]="mode">
                <div>dropDown</div>
            </robot-permissions-panel>
        </ng-container>
        <ng-container *ngIf="mode === PermissionSelectPanelModes.NORMAL">
            <robot-permissions-panel [mode]="mode"> </robot-permissions-panel>
        </ng-container>
    `,
})
class TestHostComponent {
    @ViewChild(RobotPermissionsPanelComponent)
    robotPermissionsPanelComponent: RobotPermissionsPanelComponent;
    mode = PermissionSelectPanelModes.NORMAL;
    protected readonly PermissionSelectPanelModes = PermissionSelectPanelModes;
}
