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
import { CUSTOM_ELEMENTS_SCHEMA } from '@angular/core';
import { ConfigurationComponent } from './config.component';
import { SharedTestingModule } from '../../../shared/shared.module';
import { ConfigService } from './config.service';
import { BoolValueItem, Configuration } from './config';

describe('ConfigurationComponent', () => {
    let component: ConfigurationComponent;
    let fixture: ComponentFixture<ConfigurationComponent>;
    let currentConfig: Configuration;
    const fakeConfigService = {
        getConfig() {
            return currentConfig;
        },
        getOriginalConfig() {
            return new Configuration();
        },
        getLoadingConfigStatus() {
            return false;
        },
        updateConfig() {},
    };
    let initSpy: jasmine.Spy;
    beforeEach(() => {
        TestBed.configureTestingModule({
            imports: [SharedTestingModule],
            schemas: [CUSTOM_ELEMENTS_SCHEMA],
            declarations: [ConfigurationComponent],
            providers: [
                { provide: ConfigService, useValue: fakeConfigService },
            ],
        }).compileComponents();
    });

    beforeEach(() => {
        currentConfig = new Configuration();
        initSpy = spyOn(fakeConfigService, 'updateConfig').and.returnValue(
            undefined
        );
        fixture = TestBed.createComponent(ConfigurationComponent);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });
    it('should init config', async () => {
        await fixture.whenStable();
        expect(initSpy.calls.count()).toEqual(1);
    });
    it('should expose only enabled commercial navigation', () => {
        currentConfig.enable_commercial_branding = new BoolValueItem(
            false,
            true
        );

        expect(
            component.commercialFeatureEnabled('enable_commercial_branding')
        ).toBeFalse();

        currentConfig.enable_commercial_branding.value = true;
        expect(
            component.commercialFeatureEnabled('enable_commercial_branding')
        ).toBeTrue();
    });
    it('should show identity providers only when its commercial feature is enabled', () => {
        currentConfig.enable_commercial_identity_providers = new BoolValueItem(
            false,
            true
        );
        fixture.detectChanges();

        expect(
            fixture.nativeElement.querySelector('#config-identity-providers')
        ).toBeFalsy();

        currentConfig.enable_commercial_identity_providers.value = true;
        fixture.detectChanges();

        expect(
            fixture.nativeElement.querySelector('#config-identity-providers')
        ).toBeTruthy();
    });
    it('should always expose the base audit log configuration tab', () => {
        currentConfig.enable_commercial_audit_log_otlp = new BoolValueItem(
            false,
            true
        );
        fixture.detectChanges();

        expect(
            fixture.nativeElement.querySelector('#config-audit-log')
        ).toBeTruthy();
    });
    it('should display configuration tabs alphabetically', () => {
        currentConfig.enable_commercial_branding = new BoolValueItem(
            true,
            true
        );
        currentConfig.enable_commercial_identity_providers = new BoolValueItem(
            true,
            true
        );
        fixture.detectChanges();

        const navItems = fixture.nativeElement.querySelectorAll(
            '.nav .nav-link'
        ) as NodeListOf<HTMLElement>;
        const tabIDs = Array.from(navItems, item => item.id);

        expect(tabIDs).toEqual([...tabIDs].sort());
    });
});
