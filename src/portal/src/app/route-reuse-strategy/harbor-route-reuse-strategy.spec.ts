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
import { HarborRouteReuseStrategy } from './harbor-route-reuse-strategy';
import { ActivatedRouteSnapshot } from '@angular/router';

describe('HarborRouteReuseStrategy', () => {
    let harborRouteReuseStrategy: HarborRouteReuseStrategy;
    beforeEach(() => {
        harborRouteReuseStrategy = new HarborRouteReuseStrategy();
    });
    it('should be created', () => {
        expect(harborRouteReuseStrategy).toBeTruthy();
    });
    it('shouldReuseRoute', () => {
        const future: ActivatedRouteSnapshot = new ActivatedRouteSnapshot();
        const curr: ActivatedRouteSnapshot = new ActivatedRouteSnapshot();
        expect(
            harborRouteReuseStrategy.shouldReuseRoute(future, curr)
        ).toBeTruthy();
    });
    it('reuses list routes when only the filter query changes', () => {
        const config = {};
        const snapshot = (url: string): ActivatedRouteSnapshot => {
            const s: any = new ActivatedRouteSnapshot();
            s.routeConfig = config;
            s._routerState = { url };
            return s;
        };
        const list = '/harbor/projects/1/repositories';
        expect(
            harborRouteReuseStrategy.shouldReuseRoute(
                snapshot(`${list}?name=a`),
                snapshot(`${list}?name=b`)
            )
        ).toBeTrue();
        expect(
            harborRouteReuseStrategy.shouldReuseRoute(
                snapshot(`${list}/team%2Fapp/artifacts-tab?q=tags%3D*`),
                snapshot(`${list}/team%2Fapp/artifacts-tab`)
            )
        ).toBeTrue();
        expect(
            harborRouteReuseStrategy.shouldReuseRoute(
                snapshot(list),
                snapshot(list)
            )
        ).toBeFalse();
    });
    it('does not reuse list routes across targets when a query is present', () => {
        const config = {};
        const snapshot = (url: string): ActivatedRouteSnapshot => {
            const s: any = new ActivatedRouteSnapshot();
            s.routeConfig = config;
            s._routerState = { url };
            return s;
        };
        expect(
            harborRouteReuseStrategy.shouldReuseRoute(
                snapshot('/harbor/projects/2/repositories?name=a'),
                snapshot('/harbor/projects/1/repositories?name=a')
            )
        ).toBeFalse();
        expect(
            harborRouteReuseStrategy.shouldReuseRoute(
                snapshot(
                    '/harbor/projects/1/repositories/team%2Fapp/artifacts-tab?q=tags%3D~v'
                ),
                snapshot(
                    '/harbor/projects/1/repositories/nginx/artifacts-tab?q=tags%3D~v'
                )
            )
        ).toBeFalse();
        expect(
            harborRouteReuseStrategy.shouldReuseRoute(
                snapshot(
                    '/harbor/projects/1/repositories/team%2Fapp/artifacts-tab'
                ),
                snapshot(
                    '/harbor/projects/1/repositories/nginx/artifacts-tab?q=tags%3D~v'
                )
            )
        ).toBeFalse();
        // Re-opening the same filtered list rebuilds it, dropping pending input.
        expect(
            harborRouteReuseStrategy.shouldReuseRoute(
                snapshot('/harbor/projects/1/repositories?name=a'),
                snapshot('/harbor/projects/1/repositories?name=a')
            )
        ).toBeFalse();
        // Leaving a list for another project tab keeps the shared project shell.
        expect(
            harborRouteReuseStrategy.shouldReuseRoute(
                snapshot('/harbor/projects/1/summary'),
                snapshot('/harbor/projects/1/repositories?name=a')
            )
        ).toBeTrue();
    });
});
