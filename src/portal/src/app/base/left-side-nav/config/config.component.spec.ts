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
import { Configuration } from './config';
import { AppConfigService } from '../../../services/app-config.service';
import { AppConfig } from '../../../services/app-config';

describe('ConfigurationComponent', () => {
    let component: ConfigurationComponent;
    let fixture: ComponentFixture<ConfigurationComponent>;
    const fakeConfigService = {
        getConfig() {
            return new Configuration();
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
    let appConfig: AppConfig;
    beforeEach(() => {
        TestBed.configureTestingModule({
            imports: [SharedTestingModule],
            schemas: [CUSTOM_ELEMENTS_SCHEMA],
            declarations: [ConfigurationComponent],
            providers: [
                { provide: ConfigService, useValue: fakeConfigService },
                {
                    provide: AppConfigService,
                    useValue: {
                        hasFeatures: () =>
                            Object.keys(appConfig.features || {}).length > 0,
                    },
                },
            ],
        }).compileComponents();
    });

    beforeEach(() => {
        appConfig = new AppConfig();
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
    it('hides the Features tab without registered features', () => {
        appConfig.features = {};
        fixture.detectChanges();
        expect(
            fixture.nativeElement.querySelector('#config-features')
        ).toBeNull();
    });
    it('shows the Features tab once a feature is registered', () => {
        appConfig.features = { enable_test_feature: false };
        fixture.detectChanges();
        expect(
            fixture.nativeElement.querySelector('#config-features')
        ).not.toBeNull();
    });
});
