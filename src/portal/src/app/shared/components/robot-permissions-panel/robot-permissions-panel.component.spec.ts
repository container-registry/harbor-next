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

    it('fills its row in normal mode only', async () => {
        const host = () =>
            fixture.nativeElement.querySelector('robot-permissions-panel');
        expect(host().classList).toContain('fill');
        for (const mode of [
            PermissionSelectPanelModes.DROPDOWN,
            PermissionSelectPanelModes.MODAL,
        ]) {
            component.mode = mode;
            fixture.detectChanges();
            await fixture.whenStable();
            expect(host().classList).not.toContain('fill');
        }
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

    it('keeps member:grant-admin out of the action columns', () => {
        expect(panel.candidateActions).toEqual(['create', 'pull']);
        const headers = fixture.nativeElement.querySelectorAll('thead th');
        expect(headers.length).toBe(3);
    });

    it('renders member:grant-admin as a plain sub-row of the member resource', () => {
        const rows = fixture.nativeElement.querySelectorAll('tbody tr');
        expect(rows.length).toBe(3);
        expect(rows[0].textContent).toContain('ROBOT_ACCOUNT.MEMBER');
        expect(rows[1].classList).toContain('sub-row');
        expect(rows[1].textContent).toContain('ROBOT_ACCOUNT.GRANT_ADMIN');
        expect(rows[1].textContent).not.toContain('GRANT_ADMIN_HINT');
        expect(rows[1].querySelector('clr-icon')).toBeNull();
    });

    it('puts the sub-row checkbox in the Create column', () => {
        const rows = fixture.nativeElement.querySelectorAll('tbody tr');
        const cells = rows[1].querySelectorAll('td');
        expect(cells.length).toBe(1 + panel.candidateActions.length);
        expect(panel.subRowColumn()).toBe('create');
        const createIndex = 1 + panel.candidateActions.indexOf('create');
        expect(
            cells[createIndex].querySelector('input[type="checkbox"]')
        ).toBeTruthy();
        expect(rows[1].querySelectorAll('input[type="checkbox"]').length).toBe(
            1
        );
    });

    it('shows a caret on the member row only and starts expanded', () => {
        const rows = fixture.nativeElement.querySelectorAll('tbody tr');
        expect(rows[0].querySelector('.sub-row-toggle')).toBeTruthy();
        expect(rows[2].querySelector('.sub-row-toggle')).toBeNull();
        expect(panel.isExpanded('member')).toBeTrue();
        expect(fixture.nativeElement.querySelector('.sub-row')).toBeTruthy();
    });

    it('hides the sub-row when the member row is collapsed', () => {
        fixture.nativeElement.querySelector('.sub-row-toggle').click();
        fixture.detectChanges();
        expect(panel.isExpanded('member')).toBeFalse();
        expect(fixture.nativeElement.querySelector('.sub-row')).toBeNull();
        fixture.nativeElement.querySelector('.sub-row-toggle').click();
        fixture.detectChanges();
        expect(fixture.nativeElement.querySelector('.sub-row')).toBeTruthy();
    });

    it('select all expands the member row and ticks member:grant-admin', async () => {
        panel.toggleExpanded('member');
        expect(panel.isExpanded('member')).toBeFalse();
        panel.selectAllOrUnselectAll();
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();
        expect(panel.isExpanded('member')).toBeTrue();
        expect(hasPermission(panel.permissionsModel, grantAdmin)).toBeTrue();
        expect(
            fixture.nativeElement.querySelector(
                '.sub-row input[type="checkbox"]'
            ).checked
        ).toBeTrue();
        panel.selectAllOrUnselectAll();
        expect(hasPermission(panel.permissionsModel, grantAdmin)).toBeFalse();
    });

    it('toggles member:grant-admin from its own checkbox', () => {
        panel.setCheckBoxValue(grantAdmin.resource, grantAdmin.action, true);
        expect(hasPermission(panel.permissionsModel, grantAdmin)).toBeTrue();
        expect(panel.getCheckBoxValue('member', 'grant-admin')).toBeTrue();
        panel.setCheckBoxValue(grantAdmin.resource, grantAdmin.action, false);
        expect(hasPermission(panel.permissionsModel, grantAdmin)).toBeFalse();
    });

    it('select all includes member:grant-admin', () => {
        panel.selectAllOrUnselectAll();
        expect(hasPermission(panel.permissionsModel, pull)).toBeTrue();
        expect(hasPermission(panel.permissionsModel, createMember)).toBeTrue();
        expect(hasPermission(panel.permissionsModel, grantAdmin)).toBeTrue();
        expect(panel.isAllSelected()).toBeTrue();
        panel.selectAllOrUnselectAll();
        expect(panel.permissionsModel.length).toBe(0);
    });

    it('is not all selected while member:grant-admin is unticked', () => {
        panel.permissionsModel = [pull, createMember];
        expect(panel.isAllSelected()).toBeFalse();
    });

    it('shows no sub-row when member:grant-admin is not offered', () => {
        render([pull, createMember]);
        expect(fixture.nativeElement.querySelector('.sub-row')).toBeNull();
        expect(fixture.nativeElement.querySelectorAll('tbody tr').length).toBe(
            2
        );
    });

    it('select all keeps permissions the panel does not offer', () => {
        render([pull, createMember]);
        panel.permissionsModel = [grantAdmin];
        panel.selectAllOrUnselectAll();
        expect(hasPermission(panel.permissionsModel, grantAdmin)).toBeTrue();
        expect(hasPermission(panel.permissionsModel, pull)).toBeTrue();
        expect(panel.isAllSelected()).toBeTrue();
        panel.selectAllOrUnselectAll();
        expect(panel.permissionsModel).toEqual([grantAdmin]);
    });

    it('still shows the checkbox when member:grant-admin is the only permission', () => {
        render([grantAdmin]);
        expect(panel.candidateActions).toEqual([]);
        const subRow = fixture.nativeElement.querySelector('.sub-row');
        expect(subRow.querySelectorAll('td').length).toBe(2);
        expect(subRow.querySelector('input[type="checkbox"]')).toBeTruthy();
    });

    it('names the sub-row checkbox after its label', () => {
        const checkbox = fixture.nativeElement.querySelector(
            '.sub-row input[type="checkbox"]'
        );
        expect(checkbox.getAttribute('aria-label')).toBe(
            'ROBOT_ACCOUNT.GRANT_ADMIN'
        );
    });

    it('disables every checkbox in the read-only modal', async () => {
        panel.mode = PermissionSelectPanelModes.MODAL;
        panel.modalOpen = true;
        fixture.detectChanges();
        await fixture.whenStable();
        const boxes = fixture.nativeElement.querySelectorAll(
            'input[type="checkbox"]'
        );
        expect(boxes.length).toBe(3);
        boxes.forEach(box => expect(box.disabled).toBeTrue());
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
