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
import { TestBed } from '@angular/core/testing';
import { ActivatedRouteSnapshot, Router, UrlTree } from '@angular/router';
import { RouterTestingModule } from '@angular/router/testing';
import { of, throwError } from 'rxjs';
import { FeatureGuard } from './feature-guard-activate.service';
import { AppConfigService } from '../../services/app-config.service';
import { AppConfig } from '../../services/app-config';

describe('FeatureGuard', () => {
    let guard: FeatureGuard;
    let features: { [key: string]: boolean };
    let loadFails: boolean;
    const fakeAppConfigService = {
        load: () =>
            loadFails
                ? throwError(() => new Error('down'))
                : of(new AppConfig()),
        featureEnabled: (key: string) => features[key] === true,
    };
    const route = {
        data: { feature: 'enable_model_sync' },
    } as unknown as ActivatedRouteSnapshot;

    beforeEach(() => {
        features = {};
        loadFails = false;
        TestBed.configureTestingModule({
            imports: [RouterTestingModule],
            providers: [
                { provide: AppConfigService, useValue: fakeAppConfigService },
            ],
        });
        guard = TestBed.inject(FeatureGuard);
    });

    it('opens the route while the feature is enabled', done => {
        features['enable_model_sync'] = true;
        guard.canActivate(route).subscribe(result => {
            expect(result).toBeTrue();
            done();
        });
    });

    it('redirects to the projects page while the feature is disabled', done => {
        guard.canActivate(route).subscribe(result => {
            expect(result instanceof UrlTree).toBeTrue();
            expect(TestBed.inject(Router).serializeUrl(result as UrlTree)).toBe(
                '/harbor/projects'
            );
            done();
        });
    });

    it('redirects when the system info cannot be loaded', done => {
        features['enable_model_sync'] = true;
        loadFails = true;
        guard.canActivate(route).subscribe(result => {
            expect(result instanceof UrlTree).toBeTrue();
            done();
        });
    });
});
