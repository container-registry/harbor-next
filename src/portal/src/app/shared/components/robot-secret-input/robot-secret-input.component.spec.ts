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
import { NO_ERRORS_SCHEMA } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { TranslateModule } from '@ngx-translate/core';
import { RobotSecretInputComponent } from './robot-secret-input.component';

describe('RobotSecretInputComponent', () => {
    let component: RobotSecretInputComponent;
    let fixture: ComponentFixture<RobotSecretInputComponent>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [FormsModule, TranslateModule.forRoot()],
            declarations: [RobotSecretInputComponent],
            schemas: [NO_ERRORS_SCHEMA],
        }).compileComponents();
    });

    beforeEach(() => {
        fixture = TestBed.createComponent(RobotSecretInputComponent);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should default to acceptable with no secret', () => {
        expect(component.isAcceptable()).toBeTruthy();
    });

    it('should flag a weak secret as invalid with errors', () => {
        component.onSecretInput('weak');
        expect(component.validationErrors.length).toBeGreaterThan(0);
        expect(component.isSecretValid()).toBeFalsy();
    });

    it('should accept a secret meeting all requirements', () => {
        component.onSecretInput('StrongPass123');
        expect(component.validationErrors.length).toBe(0);
        expect(component.isSecretValid()).toBeTruthy();
    });

    it('should detect mismatched confirmation', () => {
        component.onSecretInput('StrongPass123');
        component.confirmSecret = 'Different123';
        expect(component.secretsMatch()).toBeFalsy();
    });

    it('should confirm matching secrets', () => {
        component.onSecretInput('StrongPass123');
        component.confirmSecret = 'StrongPass123';
        expect(component.secretsMatch()).toBeTruthy();
    });

    it('should block acceptance when the secret is invalid', () => {
        component.onSecretInput('weak');
        component.confirmSecret = 'weak';
        expect(component.isAcceptable()).toBeFalsy();
    });

    it('should block acceptance when the confirmation does not match', () => {
        component.onSecretInput('StrongPass123');
        component.confirmSecret = 'Mismatch123';
        expect(component.isAcceptable()).toBeFalsy();
    });

    it('should accept a valid, confirmed secret', () => {
        component.onSecretInput('StrongPass123');
        component.confirmSecret = 'StrongPass123';
        expect(component.isAcceptable()).toBeTruthy();
    });

    it('should emit the secret value on input', () => {
        const emitted: string[] = [];
        component.secretChange.subscribe(v => emitted.push(v));
        component.onSecretInput('StrongPass123');
        expect(emitted).toEqual(['StrongPass123']);
    });

    it('should emit validity changes as the secret and confirmation change', () => {
        const emitted: boolean[] = [];
        component.validChange.subscribe(v => emitted.push(v));
        component.onSecretInput('weak');
        expect(emitted[emitted.length - 1]).toBeFalsy();
        component.onSecretInput('StrongPass123');
        component.confirmSecret = 'StrongPass123';
        component.onConfirmInput();
        expect(emitted[emitted.length - 1]).toBeTruthy();
    });

    it('should toggle visibility', () => {
        expect(component.showPassword).toBeFalsy();
        component.toggleVisibility();
        expect(component.showPassword).toBeTruthy();
    });

    it('should clear its own state when the bound secret is externally reset', () => {
        component.onSecretInput('StrongPass123');
        component.confirmSecret = 'StrongPass123';
        component.secret = '';
        component.ngOnChanges();
        expect(component.confirmSecret).toBe('');
        expect(component.validationErrors.length).toBe(0);
        expect(component.isDirty).toBeFalsy();
    });
});
