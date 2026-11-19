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
import { PushImageButtonComponent } from './push-image.component';
import {
    containerPushSections,
    groupPushSections,
    PUSH_SECTION_SOURCES,
    PushSection,
} from './push-sections';
import { AppConfigService } from '../../../services/app-config.service';
import { CopyInputComponent } from './copy-input.component';
import { InlineAlertComponent } from '../inline-alert/inline-alert.component';
import { ErrorHandler } from '../../units/error-handler';
import { SharedTestingModule } from '../../shared.module';
import { Component, ViewChild } from '@angular/core';

describe('containerPushSections', () => {
    it('keeps the commands the push menu always showed', () => {
        const sections = containerPushSections('c8n.io', 'bupd');
        expect(sections.map(s => s.id)).toEqual([
            'docker',
            'podman',
            'helm',
            'cnab',
        ]);
        expect(sections.map(s => s.steps.map(step => step.command))).toEqual([
            [
                'docker tag SOURCE_IMAGE[:TAG] c8n.io/bupd/REPOSITORY[:TAG]',
                'docker push c8n.io/bupd/REPOSITORY[:TAG]',
            ],
            ['podman push IMAGE_ID c8n.io/bupd/REPOSITORY[:TAG]'],
            [
                'helm package CHART_PATH',
                'helm push CHART_PACKAGE oci://c8n.io/bupd',
            ],
            [
                'cnab-to-oci push CNAB_PATH --target c8n.io/bupd/REPOSITORY[:TAG] --auto-update-bundle',
            ],
        ]);
    });
});

describe('groupPushSections', () => {
    it('keeps sections under their heading in list order', () => {
        const section = (id: string, group: string): PushSection => ({
            id,
            titleKey: id,
            group: { id: group, titleKey: group, badgeKey: `${group}-badge` },
            steps: [],
        });
        const groups = groupPushSections([
            section('a', 'first'),
            section('b', 'second'),
            section('c', 'first'),
        ]);
        expect(groups.map(g => g.titleKey)).toEqual(['first', 'second']);
        expect(groups[0].badgeKey).toEqual('first-badge');
        expect(groups.map(g => g.sections.map(s => s.id))).toEqual([
            ['a', 'c'],
            ['b'],
        ]);
    });
});

describe('PushImageButtonComponent (inline template)', () => {
    let component: TestHostComponent;
    let fixture: ComponentFixture<TestHostComponent>;

    async function settle(): Promise<void> {
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();
    }

    function commands(): string[] {
        return Array.from(
            fixture.nativeElement.querySelectorAll(
                '.command-input'
            ) as NodeListOf<HTMLInputElement>
        ).map(input => input.value.trim());
    }

    const enabledFeatures: string[] = [];
    const extra = (id: string, more: Partial<PushSection>): PushSection => ({
        id,
        titleKey: id,
        group: { id: 'extra', titleKey: 'EXTRA', badgeKey: 'BADGE' },
        steps: [{ labelKey: 'step', command: `${id} publish` }],
        ...more,
    });
    const extraSource = () => [
        extra('gated', { feature: 'gated_feature' }),
        extra('hinted', { hintKey: 'HINT' }),
    ];

    function tabs(): HTMLElement[] {
        return Array.from(
            fixture.nativeElement.querySelectorAll('a[clrVerticalNavLink]')
        );
    }

    function headings(): string[] {
        return Array.from(
            fixture.nativeElement.querySelectorAll(
                '.nav-group-text'
            ) as NodeListOf<HTMLElement>
        ).map(heading => heading.textContent.replace(/\s+/g, ' ').trim());
    }

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [SharedTestingModule],
            declarations: [
                InlineAlertComponent,
                CopyInputComponent,
                PushImageButtonComponent,
                TestHostComponent,
            ],
            providers: [
                ErrorHandler,
                {
                    provide: AppConfigService,
                    useValue: {
                        featureEnabled: (key: string) =>
                            enabledFeatures.includes(key),
                    },
                },
            ],
        }).compileComponents();
    });

    afterEach(() => {
        enabledFeatures.length = 0;
        const at = PUSH_SECTION_SOURCES.indexOf(extraSource);
        if (at >= 0) {
            PUSH_SECTION_SOURCES.splice(at, 1);
        }
    });

    beforeEach(() => {
        fixture = TestBed.createComponent(TestHostComponent);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('should be created', () => {
        expect(component).toBeTruthy();
    });

    it('should stay closed until the button is clicked', async () => {
        await settle();
        expect(tabs().length).toEqual(0);
        expect(commands().length).toEqual(0);
    });

    it('should open the dialog on the Docker tab', async () => {
        await settle();
        const el: HTMLElement = fixture.nativeElement.querySelector('button');
        expect(el).not.toBeNull();
        el.click();
        await settle();
        expect(tabs().map(tab => tab.textContent.trim())).toEqual([
            'PUSH_IMAGE.DOCKER',
            'PUSH_IMAGE.PODMAN',
            'PUSH_IMAGE.HELM',
            'PUSH_IMAGE.CNAB',
        ]);
        expect(commands()).toEqual([
            `docker tag SOURCE_IMAGE[:TAG] https://testing.harbor.com/testing/REPOSITORY[:TAG]`,
            `docker push https://testing.harbor.com/testing/REPOSITORY[:TAG]`,
        ]);
    });

    it('should show no heading for a single group', async () => {
        await settle();
        component.pushButton.open();
        await settle();
        expect(tabs().length).toEqual(4);
        expect(headings()).toEqual([]);
    });

    it('should open on the requested tab', async () => {
        await settle();
        component.pushButton.open('helm');
        await settle();
        expect(commands()).toEqual([
            'helm package CHART_PATH',
            'helm push CHART_PACKAGE oci://https://testing.harbor.com/testing',
        ]);
    });

    it('should switch commands with the tab', async () => {
        await settle();
        component.pushButton.open();
        await settle();
        tabs()[1].click();
        await settle();
        expect(commands()).toEqual([
            'podman push IMAGE_ID https://testing.harbor.com/testing/REPOSITORY[:TAG]',
        ]);
    });

    it('should switch sections with Enter and Space', async () => {
        await settle();
        component.pushButton.open();
        await settle();
        const space = new KeyboardEvent('keydown', {
            key: ' ',
            cancelable: true,
        });
        tabs()[1].dispatchEvent(space);
        await settle();
        expect(commands()[0]).toContain('podman push');
        // Space must not scroll the dialog.
        expect(space.defaultPrevented).toBeTrue();
        tabs()[2].dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }));
        await settle();
        expect(commands()[0]).toContain('helm package');
    });

    it('should fall back to the first tab for an unknown section', async () => {
        await settle();
        component.pushButton.open('homebrew');
        await settle();
        expect(commands()[0]).toContain('docker tag');
    });

    it('should offer the sections without bound inputs', () => {
        const bare = TestBed.createComponent(PushImageButtonComponent);
        bare.detectChanges();
        bare.componentInstance.open();
        expect(bare.componentInstance.sections.length).toEqual(4);
        expect(bare.componentInstance.activeId).toEqual('docker');
    });

    it('should render whatever the section list holds', async () => {
        PUSH_SECTION_SOURCES.unshift(extraSource);
        enabledFeatures.push('gated_feature');
        fixture = TestBed.createComponent(TestHostComponent);
        component = fixture.componentInstance;
        await settle();
        component.pushButton.open();
        await settle();
        expect(tabs().length).toEqual(6);
        expect(commands()).toEqual(['gated publish']);
        expect(headings()).toEqual(['EXTRA BADGE', 'PUSH_IMAGE.GROUP_OCI']);
        expect(
            fixture.nativeElement.querySelectorAll('.nav-group-text .label')
                .length
        ).toEqual(1);
        expect(tabs().some(tab => !!tab.querySelector('.label'))).toBeFalse();
        tabs()[1].click();
        await settle();
        expect(
            fixture.nativeElement.querySelector('.alert-text').textContent
        ).toContain('HINT');
    });

    it('should leave out sections whose feature is off', async () => {
        PUSH_SECTION_SOURCES.unshift(extraSource);
        fixture = TestBed.createComponent(TestHostComponent);
        component = fixture.componentInstance;
        await settle();
        component.pushButton.open();
        await settle();
        expect(tabs().length).toEqual(5);
        expect(commands()).toEqual(['hinted publish']);
    });
});

// mock a TestHostComponent for PushImageButtonComponent
@Component({
    template: ` <hbr-push-image-button
        [projectName]="'testing'"
        [registryUrl]="'https://testing.harbor.com'">
    </hbr-push-image-button>`,
})
class TestHostComponent {
    @ViewChild(PushImageButtonComponent) pushButton: PushImageButtonComponent;
}
