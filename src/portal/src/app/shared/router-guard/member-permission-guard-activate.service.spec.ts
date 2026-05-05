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
import { Router } from '@angular/router';
import { RouterTestingModule } from '@angular/router/testing';
import { Observable, of, throwError } from 'rxjs';
import { UN_LOGGED_PARAM, YES } from '../../account/sign-in/sign-in.service';
import { CommonRoutes } from '../entities/shared.const';
import { UserPermissionService, USERSTATICPERMISSION } from '../services';
import { ErrorHandler } from '../units/error-handler';
import { MemberPermissionGuard } from './member-permission-guard-activate.service';

describe('MemberPermissionGuardActivateServiceGuard', () => {
    let guard: MemberPermissionGuard;
    let router: Router;
    let errorHandler: jasmine.SpyObj<ErrorHandler>;
    let userPermissionService: jasmine.SpyObj<UserPermissionService>;

    const repositoryListPermission = {
        resource: USERSTATICPERMISSION.REPOSITORY.KEY,
        action: USERSTATICPERMISSION.REPOSITORY.VALUE.LIST,
    };
    const memberListPermission = {
        resource: USERSTATICPERMISSION.MEMBER.KEY,
        action: USERSTATICPERMISSION.MEMBER.VALUE.LIST,
    };

    beforeEach(() => {
        errorHandler = jasmine.createSpyObj('ErrorHandler', ['error']);
        userPermissionService = jasmine.createSpyObj('UserPermissionService', [
            'getPermission',
        ]);
        TestBed.configureTestingModule({
            imports: [RouterTestingModule],
            providers: [
                MemberPermissionGuard,
                {
                    provide: ErrorHandler,
                    useValue: errorHandler,
                },
                {
                    provide: UserPermissionService,
                    useValue: userPermissionService,
                },
            ],
        });
        guard = TestBed.inject(MemberPermissionGuard);
        router = TestBed.inject(Router);
        spyOn(router, 'navigate');
    });

    it('should be created', () => {
        expect(guard).toBeTruthy();
    });

    it('should check backend permissions for normal project routes', done => {
        userPermissionService.getPermission.and.returnValue(of(true));

        const result = guard.canActivate(
            createRoute(repositoryListPermission),
            {} as any
        ) as Observable<boolean>;

        result.subscribe(canActivate => {
            expect(canActivate).toBeTrue();
            expect(userPermissionService.getPermission).toHaveBeenCalledWith(
                1,
                USERSTATICPERMISSION.REPOSITORY.KEY,
                USERSTATICPERMISSION.REPOSITORY.VALUE.LIST
            );
            expect(router.navigate).not.toHaveBeenCalled();
            done();
        });
    });

    it('should allow public anonymous repository list permission without backend permission check', () => {
        const result = guard.canActivate(
            createRoute(repositoryListPermission, { [UN_LOGGED_PARAM]: YES }),
            {} as any
        );

        expect(result).toBeTrue();
        expect(userPermissionService.getPermission).not.toHaveBeenCalled();
        expect(router.navigate).not.toHaveBeenCalled();
    });

    it('should block public anonymous non-read permissions', () => {
        const result = guard.canActivate(
            createRoute(memberListPermission, { [UN_LOGGED_PARAM]: YES }),
            {} as any
        );

        expect(result).toBeFalse();
        expect(userPermissionService.getPermission).not.toHaveBeenCalled();
        expect(router.navigate).toHaveBeenCalledWith(
            [CommonRoutes.HARBOR_DEFAULT],
            { queryParams: { [UN_LOGGED_PARAM]: YES } }
        );
    });

    it('should redirect when backend permission check fails', done => {
        const error = { status: 401 };
        userPermissionService.getPermission.and.returnValue(
            throwError(() => error)
        );

        const result = guard.canActivate(
            createRoute(repositoryListPermission),
            {} as any
        ) as Observable<boolean>;

        result.subscribe(canActivate => {
            expect(canActivate).toBeFalse();
            expect(router.navigate).toHaveBeenCalledWith([
                CommonRoutes.HARBOR_DEFAULT,
            ]);
            expect(errorHandler.error).toHaveBeenCalledWith(error);
            done();
        });
    });

    function createRoute(permission, queryParams = {}) {
        return {
            parent: { params: { id: 1 } },
            data: { permissionParam: permission },
            queryParams,
        } as any;
    }
});
