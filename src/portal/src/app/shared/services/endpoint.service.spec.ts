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
import { SharedTestingModule } from '../shared.module';
import { EndpointDefaultService, EndpointService } from './endpoint.service';

describe('EndpointService', () => {
    beforeEach(() => {
        TestBed.configureTestingModule({
            imports: [SharedTestingModule],
            providers: [
                EndpointDefaultService,
                {
                    provide: EndpointService,
                    useClass: EndpointDefaultService,
                },
            ],
        });
    });

    it('should be initialized', inject(
        [EndpointDefaultService],
        (service: EndpointService) => {
            expect(service).toBeTruthy();
        }
    ));

    it('should distinguish well-known package providers from generic registries', inject(
        [EndpointDefaultService],
        (service: EndpointService) => {
            expect(service.getAdapterText('npmjs')).toBe('npmjs.org');
            expect(service.getAdapterText('npm')).toBe('npm Registry');
            expect(service.getAdapterText('maven-central')).toBe(
                'Maven Central'
            );
            expect(service.getAdapterText('maven')).toBe('Maven Registry');
            expect(service.getAdapterText('pypi')).toBe('PyPI');
            expect(service.getAdapterText('pypi-registry')).toBe(
                'PyPI Registry'
            );
            expect(service.getAdapterText('crates-io')).toBe('crates.io');
            expect(service.getAdapterText('cargo')).toBe('Cargo Registry');
            expect(service.getAdapterText('go')).toBe('Go Proxy');
            expect(service.getAdapterText('go-registry')).toBe('Go Registry');
            expect(service.getAdapterText('homebrew')).toBe('Homebrew');
            expect(service.getAdapterText('homebrew-registry')).toBe(
                'Homebrew Registry'
            );
        }
    ));
});
