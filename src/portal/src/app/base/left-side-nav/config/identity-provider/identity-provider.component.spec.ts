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
import { NO_ERRORS_SCHEMA } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { FormsModule } from '@angular/forms';
import { ClarityModule } from '@clr/angular';
import { TranslateModule } from '@ngx-translate/core';
import { of } from 'rxjs';
import { AppConfigService } from '../../../../services/app-config.service';
import { ConfigurationService } from '../../../../services/config.service';
import { MessageHandlerService } from '../../../../shared/services/message-handler.service';
import { BoolValueItem, Configuration } from '../config';
import { ConfigService } from '../config.service';
import { IdentityProviderConfigurationComponent } from './identity-provider.component';

describe('IdentityProviderConfigurationComponent', () => {
    let fixture: ComponentFixture<IdentityProviderConfigurationComponent>;
    let currentConfig: Configuration;
    let originalConfig: Configuration;

    const configurationService = {
        saveConfiguration: jasmine
            .createSpy('saveConfiguration')
            .and.returnValue(of({})),
    };
    const appConfigService = {
        load: jasmine.createSpy('load').and.returnValue(of({})),
    };
    const messageHandler = {
        showSuccess: jasmine.createSpy('showSuccess'),
        handleError: jasmine.createSpy('handleError'),
    };
    const configService = {
        resetConfig: jasmine.createSpy('resetConfig'),
        updateConfig: jasmine.createSpy('updateConfig'),
        getConfig: () => currentConfig,
        getOriginalConfig: () => originalConfig,
    };

    beforeEach(async () => {
        currentConfig = createConfig(false, true);
        originalConfig = createConfig(false, true);
        configurationService.saveConfiguration.calls.reset();
        appConfigService.load.calls.reset();
        messageHandler.showSuccess.calls.reset();
        configService.resetConfig.calls.reset();
        configService.updateConfig.calls.reset();

        await TestBed.configureTestingModule({
            imports: [TranslateModule.forRoot(), FormsModule, ClarityModule],
            schemas: [NO_ERRORS_SCHEMA],
            declarations: [IdentityProviderConfigurationComponent],
            providers: [
                {
                    provide: ConfigurationService,
                    useValue: configurationService,
                },
                { provide: AppConfigService, useValue: appConfigService },
                { provide: ConfigService, useValue: configService },
                {
                    provide: MessageHandlerService,
                    useValue: messageHandler,
                },
            ],
        }).compileComponents();

        fixture = TestBed.createComponent(
            IdentityProviderConfigurationComponent
        );
        fixture.detectChanges();
    });

    it('should render the project identity provider toggle', () => {
        const checkbox: HTMLInputElement =
            fixture.nativeElement.querySelector('#projectFedIdp');

        expect(checkbox).toBeTruthy();
        expect(checkbox.checked).toBeFalse();
        expect(checkbox.disabled).toBeFalse();
    });

    it('should disable the toggle when config is absent', () => {
        fixture.destroy();
        currentConfig = new Configuration();
        fixture = TestBed.createComponent(
            IdentityProviderConfigurationComponent
        );
        fixture.detectChanges();

        const checkbox: HTMLInputElement =
            fixture.nativeElement.querySelector('#projectFedIdp');
        expect(checkbox.disabled).toBeTrue();

        fixture.componentInstance.save();
        expect(configurationService.saveConfiguration).not.toHaveBeenCalled();
    });

    it('should disable the toggle when config is read-only', () => {
        currentConfig.enable_project_federated_idp.editable = false;
        fixture.detectChanges();

        const checkbox: HTMLInputElement =
            fixture.nativeElement.querySelector('#projectFedIdp');
        expect(checkbox.disabled).toBeTrue();

        currentConfig.enable_project_federated_idp.value = true;
        fixture.componentInstance.save();
        expect(configurationService.saveConfiguration).not.toHaveBeenCalled();
    });

    it('should save only project identity provider config', () => {
        currentConfig.enable_project_federated_idp.value = true;

        fixture.componentInstance.save();

        expect(configurationService.saveConfiguration).toHaveBeenCalledWith({
            enable_project_federated_idp: true,
        });
        expect(configService.updateConfig).toHaveBeenCalled();
        expect(appConfigService.load).toHaveBeenCalled();
    });
});

function createConfig(value: boolean, editable: boolean): Configuration {
    const config = new Configuration();
    config.enable_project_federated_idp = new BoolValueItem(value, editable);
    return config;
}
