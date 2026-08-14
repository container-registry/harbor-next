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
import { CommercialComponent } from './commercial.component';
import { TranslateModule } from '@ngx-translate/core';
import { MessageHandlerService } from '../../../../shared/services/message-handler.service';
import { ConfigurationService } from '../../../../services/config.service';
import { AppConfigService } from '../../../../services/app-config.service';
import { ConfigService } from '../config.service';
import { Configuration } from '../config';
import { SystemInfoService } from '../../../../shared/services/system-info.service';
import { of } from 'rxjs';
import { ClarityModule } from '@clr/angular';

describe('CommercialComponent', () => {
    let component: CommercialComponent;
    let fixture: ComponentFixture<CommercialComponent>;
    let mockConfig: Configuration;
    let mockOriginalConfig: Configuration;

    const mockMessageHandlerService = {
        error: () => {},
        showSuccess: () => {},
    };

    const mockConfigurationService = {
        getConfiguration: () => of({}),
    };

    const mockAppConfigService = {
        getConfig: () => ({}),
    };

    const mockConfigService = {
        resetConfig: () => {},
        getConfig: () => mockConfig,
        getLoadingConfigStatus: () => false,
        getOriginalConfig: () => mockOriginalConfig,
    };

    const mockSystemInfoService = {
        getSystemInfo: () => of({ external_url: 'http://localhost' }),
    };

    beforeEach(async () => {
        mockConfig = commercialConfig(false, true);
        mockOriginalConfig = commercialConfig(false, true);

        await TestBed.configureTestingModule({
            imports: [TranslateModule.forRoot(), FormsModule, ClarityModule],
            schemas: [NO_ERRORS_SCHEMA],
            declarations: [CommercialComponent],
            providers: [
                {
                    provide: MessageHandlerService,
                    useValue: mockMessageHandlerService,
                },
                {
                    provide: ConfigurationService,
                    useValue: mockConfigurationService,
                },
                {
                    provide: AppConfigService,
                    useValue: mockAppConfigService,
                },
                { provide: ConfigService, useValue: mockConfigService },
                {
                    provide: SystemInfoService,
                    useValue: mockSystemInfoService,
                },
            ],
        }).compileComponents();
    });

    function createComponent(): void {
        fixture = TestBed.createComponent(CommercialComponent);
        component = fixture.componentInstance;
        fixture.detectChanges();
    }

    beforeEach(() => {
        createComponent();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should render the project federated identity provider toggle', () => {
        const checkbox: HTMLInputElement =
            fixture.nativeElement.querySelector('#projectFedIdp');

        expect(checkbox).toBeTruthy();
        expect(checkbox.checked).toBeFalsy();
        expect(checkbox.disabled).toBeFalsy();
    });

    it('should disable the toggle when the backend marks it read-only', async () => {
        fixture.destroy();
        mockConfig = commercialConfig(false, false);
        mockOriginalConfig = commercialConfig(false, false);
        createComponent();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(
            (component.currentConfig as any).enable_project_federated_idp
                .editable
        ).toBeFalse();

        const checkbox: HTMLInputElement =
            fixture.nativeElement.querySelector('#projectFedIdp');
        expect(checkbox.disabled).toBeTruthy();
    });

    it('should include project federated identity provider changes in the save payload', () => {
        (mockConfig as any).enable_project_federated_idp.value = true;

        expect(component.getChanges()).toEqual({
            enable_project_federated_idp: true,
        });
    });
});

function commercialConfig(value: boolean, editable: boolean): Configuration {
    const config = new Configuration();
    (config as any).enable_project_federated_idp = {
        value,
        editable,
    };
    return config;
}
