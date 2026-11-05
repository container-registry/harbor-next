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
import { NEVER, Observable, of, throwError } from 'rxjs';
import { ConfigureService } from 'ng-swagger-gen/services/configure.service';
import { FeatureList } from 'ng-swagger-gen/models/feature-list';
import { SharedTestingModule } from '../../../../shared/shared.module';
import { AppConfigService } from '../../../../services/app-config.service';
import { MessageHandlerService } from '../../../../shared/services/message-handler.service';
import { FeaturesComponent } from './features.component';

function featureList(): FeatureList {
    return {
        sections: [
            {
                id: 'licensed',
                title: 'Licensed Features',
                description: 'Needs an agreement.',
                link_text: 'Request a license',
                link: 'mailto:sales@example.com?body=Features%3A%20none',
            },
            { id: 'optional', title: 'CONFIG.OPTIONAL_FEATURES' },
        ],
        features: [
            {
                key: 'enable_test_feature',
                section: 'optional',
                name: 'Test Feature',
                description: 'A feature used by the spec.',
                beta: true,
                enabled: false,
                editable: true,
                locked_by: '',
            },
            {
                key: 'enable_locked_feature',
                section: 'optional',
                name: 'Locked Feature',
                enabled: true,
                editable: false,
                locked_by: 'HARBOR_ENABLE_LOCKED_FEATURE',
            },
            {
                key: 'enable_licensed_feature',
                section: 'licensed',
                name: 'Licensed Feature',
                enabled: false,
                editable: true,
            },
        ],
    };
}

describe('FeaturesComponent', () => {
    let fixture: ComponentFixture<FeaturesComponent>;
    let component: FeaturesComponent;
    let listFeatures: jasmine.Spy;
    let updateFeature: jasmine.Spy;
    let appConfigLoad: jasmine.Spy;

    function setup(list: Observable<FeatureList> = of(featureList())): void {
        listFeatures = jasmine.createSpy('listFeatures').and.returnValue(list);
        updateFeature = jasmine
            .createSpy('updateFeature')
            .and.returnValue(of(null));
        appConfigLoad = jasmine.createSpy('load').and.returnValue(of({}));
        TestBed.configureTestingModule({
            imports: [SharedTestingModule],
            declarations: [FeaturesComponent],
            providers: [
                {
                    provide: ConfigureService,
                    useValue: { listFeatures, updateFeature },
                },
                {
                    provide: AppConfigService,
                    useValue: { load: appConfigLoad },
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

    function toggle(key: string): HTMLInputElement {
        return fixture.nativeElement.querySelector(`input#${key}`);
    }

    async function render(): Promise<void> {
        await fixture.whenStable();
        fixture.detectChanges();
    }

    it('renders the sections in API order with their features', async () => {
        setup();
        await render();
        const sections = Array.from(
            fixture.nativeElement.querySelectorAll('form section')
        ) as HTMLElement[];
        expect(sections.map(section => section.id)).toEqual([
            'feature-section-licensed',
            'feature-section-optional',
        ]);
        expect(sections[0].textContent).toContain('Licensed Feature');
        expect(sections[0].textContent).not.toContain('Test Feature');
        const link: HTMLAnchorElement =
            sections[0].querySelector('a.section-link');
        expect(link.textContent.trim()).toBe('Request a license');
        expect(link.getAttribute('href')).toBe(
            'mailto:sales@example.com?body=Features%3A%20none'
        );
        expect(sections[1].querySelector('.beta-badge')).not.toBeNull();
        expect(sections[1].querySelector('a.section-link')).toBeNull();
    });

    it('locks a feature the backend reports as not editable', async () => {
        setup();
        await render();
        expect(toggle('enable_locked_feature').disabled).toBeTrue();
        expect(toggle('enable_locked_feature').checked).toBeTrue();
        expect(toggle('enable_test_feature').disabled).toBeFalse();
    });

    it('saves only changed features and reloads the list and systeminfo', () => {
        setup();
        const [testFeature, , licensed] = component.features;
        component.setEnabled(testFeature, true);
        component.setEnabled(licensed, true);
        component.setEnabled(licensed, false);
        expect(component.hasChanges()).toBeTrue();

        component.save();

        expect(updateFeature).toHaveBeenCalledOnceWith({
            featureKey: 'enable_test_feature',
            feature: { enabled: true },
        });
        expect(listFeatures).toHaveBeenCalledTimes(2);
        expect(appConfigLoad).toHaveBeenCalled();
        expect(component.hasChanges()).toBeFalse();
    });

    it('reloads after a rejected change so the stored state is shown', () => {
        setup();
        updateFeature.and.returnValue(
            throwError(() => new Error('controlled by env'))
        );
        component.setEnabled(component.features[0], true);
        component.save();
        expect(listFeatures).toHaveBeenCalledTimes(2);
        expect(component.hasChanges()).toBeFalse();
    });

    it('cancel drops pending changes', () => {
        setup();
        component.setEnabled(component.features[0], true);
        component.cancel();
        expect(component.hasChanges()).toBeFalse();
        expect(component.value(component.features[0])).toBeFalse();
    });

    it('keeps Save, Cancel and toggles disabled while the list loads', () => {
        setup(NEVER);
        component.features = featureList().features;
        component.setEnabled(component.features[0], true);
        fixture.detectChanges();
        expect(
            fixture.nativeElement.querySelector('#features-save').disabled
        ).toBeTrue();
        expect(
            fixture.nativeElement.querySelector('#features-cancel').disabled
        ).toBeTrue();
        expect(component.disabled(component.features[0])).toBeTrue();
    });

    ['clr-icon', '.beta-badge'].forEach(hint =>
        it(`does not flip the toggle when ${hint} in the label is clicked`, async () => {
            setup();
            await render();
            (
                fixture.nativeElement.querySelector(
                    `label[for=enable_test_feature] .label-hints ${hint}`
                ) as HTMLElement
            ).click();
            await render();
            expect(toggle('enable_test_feature').checked).toBeFalse();
            expect(component.hasChanges()).toBeFalse();
        })
    );

    it('still flips the toggle when the feature name is clicked', async () => {
        setup();
        await render();
        (
            fixture.nativeElement.querySelector(
                'label[for=enable_test_feature]'
            ) as HTMLElement
        ).click();
        await render();
        expect(toggle('enable_test_feature').checked).toBeTrue();
        expect(component.hasChanges()).toBeTrue();
    });
});
