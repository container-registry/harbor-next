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
import { HttpErrorResponse } from '@angular/common/http';
import { ActivatedRouteSnapshot, Router } from '@angular/router';
import { throwError } from 'rxjs';
import { SessionService } from '../../shared/services/session.service';
import { ProjectRoutingResolver } from './project-routing-resolver.service';
import { RouterTestingModule } from '@angular/router/testing';
import { ProjectService } from '../../shared/services';

describe('ProjectRoutingResolverService', () => {
    beforeEach(() => {
        TestBed.configureTestingModule({
            imports: [RouterTestingModule],
            providers: [
                ProjectRoutingResolver,
                { provide: SessionService, useValue: null },
                {
                    provide: ProjectService,
                    useValue: {
                        getProjectFromCache: () =>
                            throwError(new HttpErrorResponse({ status: 404 })),
                    },
                },
            ],
        });
    });

    it('should be created', inject(
        [ProjectRoutingResolver],
        (service: ProjectRoutingResolver) => {
            expect(service).toBeTruthy();
        }
    ));

    it('sends the user to the project list when the project lookup fails', () => {
        const navigate = spyOn(TestBed.inject(Router), 'navigate');
        const route = {
            params: { id: '9' },
        } as unknown as ActivatedRouteSnapshot;
        let result: any = 'unset';
        let failed = false;
        TestBed.inject(ProjectRoutingResolver)
            .resolve(route, null)
            .subscribe({
                next: p => (result = p),
                error: () => (failed = true),
            });
        expect(failed).toBeFalse();
        expect(result).toBeNull();
        expect(navigate).toHaveBeenCalledWith(['/harbor', 'projects']);
    });
});
