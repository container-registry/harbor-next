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
    ComponentFixture,
    TestBed,
    fakeAsync,
    tick,
} from '@angular/core/testing';
import { SystemSettingsComponent } from './system-settings.component';
import { ErrorHandler } from '../../../../shared/units/error-handler';
import { of, throwError } from 'rxjs';
import { Configuration } from '../config';
import { SharedTestingModule } from '../../../../shared/shared.module';
import { ConfigService } from '../config.service';
import { AppConfigService } from '../../../../services/app-config.service';
import { ProjectService } from '../../../../../../ng-swagger-gen/services/project.service';

describe('SystemSettingsComponent', () => {
    let component: SystemSettingsComponent;
    let fixture: ComponentFixture<SystemSettingsComponent>;
    const fakeConfigService = {
        config: new Configuration(),
        getConfig() {
            return this.config;
        },
        setConfig(c) {
            this.config = c;
        },
        getOriginalConfig() {
            return new Configuration();
        },
        getLoadingConfigStatus() {
            return false;
        },
        confirmUnsavedChanges() {},
        updateConfig() {},
        resetConfig() {},
        saveConfiguration() {
            return of(null);
        },
    };
    const fakedAppConfigService = {
        getConfig() {
            return {};
        },
        load() {
            return of(null);
        },
    };
    const fakeProjectService = {
        listProjects(params: { q: string }) {
            const q = decodeURIComponent(params.q);
            const projects = [{ name: 'library' }, { name: 'catchall' }];
            if (q.startsWith('name=~')) {
                const term = q.substring('name=~'.length);
                return of(projects.filter(p => p.name.includes(term)));
            }
            return of(projects.filter(p => `name=${p.name}` === q));
        },
    };
    const fakedErrorHandler = {
        error() {
            return undefined;
        },
    };
    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [SharedTestingModule],
            providers: [
                { provide: AppConfigService, useValue: fakedAppConfigService },
                { provide: ErrorHandler, useValue: fakedErrorHandler },
                { provide: ConfigService, useValue: fakeConfigService },
                { provide: ProjectService, useValue: fakeProjectService },
            ],
            declarations: [SystemSettingsComponent],
        }).compileComponents();
    });
    beforeEach(() => {
        fixture = TestBed.createComponent(SystemSettingsComponent);
        component = fixture.componentInstance;
        fixture.autoDetectChanges(true);
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });
    it('cancel button should work', () => {
        const spy: jasmine.Spy = spyOn(component, 'cancel').and.returnValue(
            undefined
        );
        const cancel: HTMLButtonElement = fixture.nativeElement.querySelector(
            '#config_system_cancel'
        );
        cancel.dispatchEvent(new Event('click'));
        expect(spy.calls.count()).toEqual(1);
    });
    it('save button should work', () => {
        const input = fixture.nativeElement.querySelector('#robotNamePrefix');
        input.value = 'test';
        input.dispatchEvent(new Event('input'));
        const save: HTMLButtonElement = fixture.nativeElement.querySelector(
            '#config_system_save'
        );
        save.dispatchEvent(new Event('click'));
        expect(input.value).toEqual('test');
    });
    it('should include default project name in system changes', () => {
        const changes = component.getSystemChanges({
            default_project_name: 'catchall',
        });
        expect(changes['default_project_name']).toEqual('catchall');
    });
    it('should reject an invalid default project name and allow empty', async () => {
        await fixture.whenStable();
        const input = fixture.nativeElement.querySelector(
            '#defaultProjectName'
        );
        input.value = 'Bad Name';
        input.dispatchEvent(new Event('input'));
        await fixture.whenStable();
        const control =
            component.systemSettingsForm.controls['defaultProjectName'];
        expect(control.valid).toBeFalse();
        input.value = '';
        input.dispatchEvent(new Event('input'));
        await fixture.whenStable();
        expect(control.valid).toBeTrue();
    });
    it('should only accept an existing default project', fakeAsync(() => {
        component.handleDefaultProjectInput('nope');
        expect(component.defaultProjectChecking).toBeTrue();
        tick(300);
        expect(component.defaultProjectChecking).toBeFalse();
        expect(component.defaultProjectExists).toBeFalse();

        component.handleDefaultProjectInput('cat');
        tick(300);
        expect(component.defaultProjectExists).toBeFalse();
        expect(component.searchedProjects.map(p => p.name)).toEqual([
            'catchall',
        ]);

        component.selectDefaultProject('catchall');
        expect(component.currentConfig.default_project_name.value).toEqual(
            'catchall'
        );
        expect(component.defaultProjectExists).toBeTrue();
        expect(component.searchedProjects).toEqual([]);

        component.handleDefaultProjectInput('');
        expect(component.defaultProjectExists).toBeTrue();
        expect(component.defaultProjectChecking).toBeFalse();
        tick(300);
    }));
    it('should ignore a lookup answer for a value that changed', fakeAsync(() => {
        component.handleDefaultProjectInput('nope');
        component.handleDefaultProjectInput('');
        tick(300);
        expect(component.defaultProjectExists).toBeTrue();
        expect(component.defaultProjectChecking).toBeFalse();
    }));
    it('should report a failed lookup and leave the check to the backend', fakeAsync(() => {
        const errorSpy = spyOn(component['errorHandler'], 'error');
        spyOn(fakeProjectService, 'listProjects').and.returnValue(
            throwError(() => new Error('boom'))
        );
        component.handleDefaultProjectInput('library');
        tick(300);
        expect(errorSpy).toHaveBeenCalled();
        expect(component.defaultProjectExists).toBeTrue();
        expect(component.defaultProjectChecking).toBeFalse();
    }));
    it('should flag a stored default project that no longer exists', () => {
        component.currentConfig.default_project_name.value = 'gone';
        component.checkStoredDefaultProject();
        expect(component.defaultProjectExists).toBeFalse();

        // a reset back to an existing value clears the error without a new lookup
        component.currentConfig.default_project_name.value = 'library';
        expect(component.defaultProjectExists).toBeTrue();
        expect(component.defaultProjectChecking).toBeFalse();
    });
    it('should clear a stale missing marker when the stored project is found again', () => {
        component.currentConfig.default_project_name.value = 'gone';
        component.checkStoredDefaultProject();
        expect(component.defaultProjectExists).toBeFalse();

        component.currentConfig.default_project_name.value = 'library';
        component.missingDefaultProjects.add('library');
        component.checkStoredDefaultProject();
        expect(component.defaultProjectExists).toBeTrue();
    });
    it('should render suggestions as keyboard-focusable buttons', fakeAsync(() => {
        component.handleDefaultProjectInput('cat');
        tick(300);
        fixture.detectChanges();
        const option: HTMLButtonElement = fixture.nativeElement.querySelector(
            '.project-options button'
        );
        expect(option.textContent.trim()).toEqual('catchall');
        option.click();
        expect(component.currentConfig.default_project_name.value).toEqual(
            'catchall'
        );
    }));
    it('should keep a restored missing default invalid after an unrelated lookup', fakeAsync(() => {
        component.currentConfig.default_project_name.value = 'gone';
        component.checkStoredDefaultProject();
        expect(component.defaultProjectExists).toBeFalse();

        component.handleDefaultProjectInput('');
        tick(300);
        component.handleDefaultProjectInput('catchall');
        tick(300);
        expect(component.defaultProjectExists).toBeTrue();

        // Cancel restores the stored value through resetConfig, without a new lookup
        component.currentConfig.default_project_name.value = 'gone';
        expect(component.defaultProjectExists).toBeFalse();
    }));
});
