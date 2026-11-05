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
import { of } from 'rxjs';
import { SharedTestingModule } from '../../../../shared/shared.module';
import { ConfigurationService } from '../../../../services/config.service';
import { MessageHandlerService } from '../../../../shared/services/message-handler.service';
import { BoolValueItem, Configuration } from '../config';
import { ConfigService } from '../config.service';
import { FeaturesComponent } from './features.component';
import { OPTIONAL_FEATURES } from './optional-features';
import { FeatureToggle } from './feature-toggle';

const TEST_FEATURE: FeatureToggle = {
    configKey: 'enable_test_feature',
    envKey: 'HARBOR_ENABLE_TEST_FEATURE',
    name: 'Test Feature',
    description: 'A feature used by the spec.',
    beta: true,
};

describe('FeaturesComponent', () => {
    let fixture: ComponentFixture<FeaturesComponent>;
    let component: FeaturesComponent;
    let original: Configuration;
    let current: Configuration;
    let saveConfiguration: jasmine.Spy;
    let updateConfig: jasmine.Spy;

    function setup(features: FeatureToggle[], value?: BoolValueItem): void {
        OPTIONAL_FEATURES.splice(0, OPTIONAL_FEATURES.length, ...features);
        original = new Configuration();
        if (value) {
            original[TEST_FEATURE.configKey] = value;
        }
        current = JSON.parse(JSON.stringify(original));
        saveConfiguration = jasmine
            .createSpy('saveConfiguration')
            .and.returnValue(of(null));
        updateConfig = jasmine.createSpy('updateConfig');
        TestBed.configureTestingModule({
            imports: [SharedTestingModule],
            declarations: [FeaturesComponent],
            providers: [
                {
                    provide: ConfigService,
                    useValue: {
                        getConfig: () => current,
                        getOriginalConfig: () => original,
                        resetConfig: () => undefined,
                        updateConfig,
                    },
                },
                {
                    provide: ConfigurationService,
                    useValue: { saveConfiguration },
                },
                {
                    provide: MessageHandlerService,
                    useValue: {
                        showSuccess: () => undefined,
                        handleError: () => undefined,
                    },
                },
            ],
        }).compileComponents();
        fixture = TestBed.createComponent(FeaturesComponent);
        component = fixture.componentInstance;
        fixture.detectChanges();
    }

    function toggle(): HTMLInputElement {
        return fixture.nativeElement.querySelector(
            `input#${TEST_FEATURE.configKey}`
        );
    }

    afterEach(() => OPTIONAL_FEATURES.splice(0, OPTIONAL_FEATURES.length));

    it('renders no optional section without registered features', () => {
        setup([]);
        expect(
            fixture.nativeElement.querySelector('#optional-features')
        ).toBeNull();
    });

    it('lists an optional feature with its beta badge', async () => {
        setup([TEST_FEATURE], new BoolValueItem(false, true));
        await fixture.whenStable();
        fixture.detectChanges();
        const section: HTMLElement =
            fixture.nativeElement.querySelector('#optional-features');
        expect(section.textContent).toContain('Test Feature');
        expect(section.querySelector('.beta-badge')).not.toBeNull();
        expect(toggle().disabled).toBeFalse();
        expect(toggle().checked).toBeFalse();
    });

    it('saves only the changed feature key', () => {
        setup([TEST_FEATURE], new BoolValueItem(false, true));
        expect(component.hasChanges()).toBeFalse();

        component.setEnabled(TEST_FEATURE, true);
        expect(component.hasChanges()).toBeTrue();
        component.save();

        expect(saveConfiguration).toHaveBeenCalledOnceWith({
            [TEST_FEATURE.configKey]: true,
        });
        expect(updateConfig).toHaveBeenCalled();
    });

    it('locks a feature controlled by its environment variable', async () => {
        setup([TEST_FEATURE], new BoolValueItem(true, false));
        await fixture.whenStable();
        fixture.detectChanges();
        expect(toggle().disabled).toBeTrue();
        expect(component.lockedBy(TEST_FEATURE)).toBe(TEST_FEATURE.envKey);
    });

    it('names CONFIG_OVERWRITE_JSON when every setting is locked', () => {
        setup([TEST_FEATURE], new BoolValueItem(true, false));
        current.token_expiration.editable = false;
        expect(component.lockedBy(TEST_FEATURE)).toBe('CONFIG_OVERWRITE_JSON');
    });

    it('treats a feature missing from the API response as disabled', () => {
        setup([TEST_FEATURE]);
        expect(component.disabled(TEST_FEATURE)).toBeTrue();
        expect(component.lockedBy(TEST_FEATURE)).toBe('');
        expect(component.hasChanges()).toBeFalse();
    });
});
