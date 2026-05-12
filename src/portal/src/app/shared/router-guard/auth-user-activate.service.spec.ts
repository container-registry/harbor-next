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
import { Router, ActivatedRouteSnapshot, RouterStateSnapshot } from '@angular/router';
import { RouterTestingModule } from '@angular/router/testing';
import { SessionService } from '../services/session.service';
import { AppConfigService } from '../../services/app-config.service';
import { MessageHandlerService } from '../services/message-handler.service';
import { SearchTriggerService } from '../components/global-search/search-trigger.service';
import { AuthCheckGuard } from './auth-user-activate.service';
import { AppConfig } from '../../services/app-config';
import { Observable, throwError } from 'rxjs';
import { UN_LOGGED_PARAM, YES } from '../../account/sign-in/sign-in.service';
import { CommonRoutes, LANDING_PAGE } from '../entities/shared.const';

describe('AuthCheckGuard', () => {
    let guard: AuthCheckGuard;
    let router: Router;
    let sessionService: jasmine.SpyObj<SessionService>;
    let appConfigService: jasmine.SpyObj<AppConfigService>;
    let messageHandlerService: jasmine.SpyObj<MessageHandlerService>;
    let searchTriggerService: jasmine.SpyObj<SearchTriggerService>;

    function createRouteSnapshot(queryParams: any = {}): ActivatedRouteSnapshot {
        return { queryParams } as ActivatedRouteSnapshot;
    }

    function createStateSnapshot(url: string): RouterStateSnapshot {
        return { url } as RouterStateSnapshot;
    }

    function createAppConfig(overrides: Partial<AppConfig> = {}): AppConfig {
        const config = new AppConfig();
        return Object.assign(config, overrides);
    }

    beforeEach(() => {
        sessionService = jasmine.createSpyObj('SessionService', ['getCurrentUser', 'retrieveUser']);
        appConfigService = jasmine.createSpyObj('AppConfigService', ['getConfig']);
        messageHandlerService = jasmine.createSpyObj('MessageHandlerService', ['clear']);
        searchTriggerService = jasmine.createSpyObj('SearchTriggerService', ['closeSearch']);

        TestBed.configureTestingModule({
            imports: [RouterTestingModule],
            providers: [
                AuthCheckGuard,
                { provide: SessionService, useValue: sessionService },
                { provide: AppConfigService, useValue: appConfigService },
                { provide: MessageHandlerService, useValue: messageHandlerService },
                { provide: SearchTriggerService, useValue: searchTriggerService },
            ],
        });

        guard = TestBed.inject(AuthCheckGuard);
        router = TestBed.inject(Router);
        spyOn(router, 'navigate');
        spyOn(router, 'navigateByUrl');
    });

    it('should be created', () => {
        expect(guard).toBeTruthy();
    });

    describe('unauthenticated user routing', () => {
        beforeEach(() => {
            sessionService.getCurrentUser.and.returnValue(null);
            sessionService.retrieveUser.and.returnValue(throwError(() => new Error('not authenticated')));
        });

        [
            { name: 'landing_page = login', landingPage: LANDING_PAGE.LOGIN },
            { name: 'landing_page is empty', landingPage: '' },
        ].forEach(({ name, landingPage }) => {
            it(`should redirect to sign-in page when ${name}`, (done) => {
                appConfigService.getConfig.and.returnValue(createAppConfig({
                    unauthenticated_landing_page: landingPage,
                }));

                const route = createRouteSnapshot();
                const state = createStateSnapshot(CommonRoutes.HARBOR_DEFAULT);

                const result = guard.canActivate(route, state);
                (result as Observable<boolean>).subscribe(canActivate => {
                    expect(canActivate).toBeFalse();
                    expect(router.navigate).toHaveBeenCalledWith(
                        [CommonRoutes.EMBEDDED_SIGN_IN],
                        jasmine.objectContaining({
                            queryParams: { redirect_url: CommonRoutes.HARBOR_DEFAULT },
                        })
                    );
                    done();
                });
            });
        });

        it('should redirect to public projects page when landing_page = public_projects', (done) => {
            appConfigService.getConfig.and.returnValue(createAppConfig({
                unauthenticated_landing_page: LANDING_PAGE.PUBLIC_PROJECTS,
            }));

            const route = createRouteSnapshot();
            const state = createStateSnapshot('/harbor/users');

            const result = guard.canActivate(route, state);
            (result as Observable<boolean>).subscribe(canActivate => {
                expect(canActivate).toBeFalse();
                expect(router.navigate).toHaveBeenCalledWith(
                    [CommonRoutes.HARBOR_DEFAULT],
                    jasmine.objectContaining({
                        queryParams: { [UN_LOGGED_PARAM]: YES },
                    })
                );
                done();
            });
        });

        it('should allow public projects route when already on /harbor/projects', (done) => {
            appConfigService.getConfig.and.returnValue(createAppConfig({
                unauthenticated_landing_page: LANDING_PAGE.PUBLIC_PROJECTS,
            }));

            const route = createRouteSnapshot();
            const state = createStateSnapshot(CommonRoutes.HARBOR_DEFAULT);

            const result = guard.canActivate(route, state);
            (result as Observable<boolean>).subscribe(canActivate => {
                expect(canActivate).toBeFalse();
                const urlTree = (router.navigateByUrl as jasmine.Spy).calls
                    .mostRecent()
                    .args[0];
                expect(router.serializeUrl(urlTree)).toBe(
                    `${CommonRoutes.HARBOR_DEFAULT}?${UN_LOGGED_PARAM}=${YES}`
                );
                expect(router.navigate).not.toHaveBeenCalled();
                done();
            });
        });

        it('should preserve public project repository route for unauthenticated access', (done) => {
            appConfigService.getConfig.and.returnValue(createAppConfig({
                unauthenticated_landing_page: LANDING_PAGE.PUBLIC_PROJECTS,
            }));

            const route = createRouteSnapshot();
            const state = createStateSnapshot(
                '/harbor/projects/123/repositories/library%2Fnginx/artifacts-tab'
            );

            const result = guard.canActivate(route, state);
            (result as Observable<boolean>).subscribe(canActivate => {
                expect(canActivate).toBeFalse();
                const urlTree = (router.navigateByUrl as jasmine.Spy).calls
                    .mostRecent()
                    .args[0];
                const expectedUrl =
                    '/harbor/projects/123/repositories/library%2Fnginx/artifacts-tab' +
                    '?publicAndNotLogged=yes';
                expect(router.serializeUrl(urlTree)).toBe(
                    expectedUrl
                );
                expect(router.navigate).not.toHaveBeenCalled();
                done();
            });
        });

        it('should allow access when publicAndNotLogged=yes query param is set', (done) => {
            const route = createRouteSnapshot({ [UN_LOGGED_PARAM]: YES });
            const state = createStateSnapshot(CommonRoutes.HARBOR_DEFAULT);

            const result = guard.canActivate(route, state);
            (result as Observable<boolean>).subscribe(canActivate => {
                expect(canActivate).toBeTrue();
                expect(router.navigate).not.toHaveBeenCalled();
                done();
            });
        });
    });
});
