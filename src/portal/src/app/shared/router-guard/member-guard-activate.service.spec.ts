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
import { TestBed, inject } from '@angular/core/testing';
import { RouterTestingModule } from '@angular/router/testing';
import { SessionService } from '../services/session.service';
import { MemberGuard } from './member-guard-activate.service';
import { ProjectService } from '../services';
import { Router } from '@angular/router';
import { Observable, of, throwError } from 'rxjs';
import { CommonRoutes } from '../entities/shared.const';
import { UN_LOGGED_PARAM, YES } from '../../account/sign-in/sign-in.service';

describe('MemberGuard', () => {
    let sessionService: jasmine.SpyObj<SessionService>;
    let projectService: jasmine.SpyObj<ProjectService>;
    let router: Router;

    beforeEach(() => {
        sessionService = jasmine.createSpyObj('SessionService', [
            'clear',
            'getCurrentUser',
            'retrieveUser',
            'setProjectMembers',
        ]);
        projectService = jasmine.createSpyObj('ProjectService', [
            'getProjectFromCache',
        ]);
        TestBed.configureTestingModule({
            imports: [RouterTestingModule],
            providers: [
                MemberGuard,
                { provide: SessionService, useValue: sessionService },
                { provide: ProjectService, useValue: projectService },
            ],
        });
        router = TestBed.inject(Router);
        spyOn(router, 'navigate');
    });

    it('should ...', inject([MemberGuard], (guard: MemberGuard) => {
        expect(guard).toBeTruthy();
    }));

    it('should retrieve user session before opening public project route', inject(
        [MemberGuard],
        (guard: MemberGuard) => {
            sessionService.getCurrentUser.and.returnValue(null);
            sessionService.retrieveUser.and.returnValue(of({} as any));
            projectService.getProjectFromCache.and.returnValue(
                of({ project_id: 1, name: 'library' } as any)
            );

            const result = guard.canActivate(
                {
                    params: { id: 1 },
                    queryParams: { [UN_LOGGED_PARAM]: YES },
                } as any,
                {
                    url: `/harbor/projects/1/repositories?${UN_LOGGED_PARAM}=${YES}`,
                } as any
            ) as Observable<boolean>;

            result.subscribe(canActivate => {
                expect(canActivate).toBeTrue();
                expect(sessionService.retrieveUser).toHaveBeenCalled();
                expect(projectService.getProjectFromCache).toHaveBeenCalledWith(
                    1
                );
                expect(router.navigate).not.toHaveBeenCalled();
            });
        }
    ));

    it('should fall back to public project route when user retrieval fails with public marker', inject(
        [MemberGuard],
        (guard: MemberGuard) => {
            sessionService.getCurrentUser.and.returnValue(null);
            sessionService.retrieveUser.and.returnValue(
                throwError(() => ({ status: 401 }))
            );
            projectService.getProjectFromCache.and.returnValue(
                of({ project_id: 1, name: 'library' } as any)
            );

            const result = guard.canActivate(
                {
                    params: { id: 1 },
                    queryParams: { [UN_LOGGED_PARAM]: YES },
                } as any,
                {
                    url: `/harbor/projects/1/repositories?${UN_LOGGED_PARAM}=${YES}`,
                } as any
            ) as Observable<boolean>;

            result.subscribe(canActivate => {
                expect(canActivate).toBeTrue();
                expect(sessionService.retrieveUser).toHaveBeenCalled();
                expect(projectService.getProjectFromCache).toHaveBeenCalledWith(
                    1
                );
                expect(router.navigate).not.toHaveBeenCalled();
            });
        }
    ));

    it('should redirect anonymous non-public route to projects when session retrieval fails', inject(
        [MemberGuard],
        (guard: MemberGuard) => {
            sessionService.getCurrentUser.and.returnValue(null);
            sessionService.retrieveUser.and.returnValue(
                throwError(() => ({ status: 401 }))
            );

            const result = guard.canActivate(
                { params: { id: 1 }, queryParams: {} } as any,
                { url: '/harbor/projects/1/repositories' } as any
            ) as Observable<boolean>;

            result.subscribe(canActivate => {
                expect(canActivate).toBeFalse();
                expect(projectService.getProjectFromCache).not.toHaveBeenCalled();
                expect(router.navigate).toHaveBeenCalledWith([
                    CommonRoutes.HARBOR_DEFAULT,
                ]);
            });
        }
    ));
});
