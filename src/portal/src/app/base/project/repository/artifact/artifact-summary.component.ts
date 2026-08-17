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
import { Component, EventEmitter, OnInit, Output } from '@angular/core';
import { Artifact } from '../../../../../../ng-swagger-gen/models/artifact';
import { Label } from '../../../../../../ng-swagger-gen/models/label';
import { ActivatedRoute, Router } from '@angular/router';
import { Project } from '../../project';
import {
    artifactBootc,
    artifactCargo,
    artifactDefault,
    artifactHomebrew,
    artifactMaven,
    artifactNpm,
    artifactPypi,
    isBootcArtifact,
    isCargoArtifact,
    isHomebrewArtifact,
    isMavenArtifact,
    isNpmArtifact,
    isPypiArtifact,
} from './artifact';
import { SafeUrl } from '@angular/platform-browser';
import { ArtifactService } from './artifact.service';
import {
    EventService,
    HarborEvent,
} from '../../../../services/event-service/event.service';
import { AppConfigService } from '../../../../services/app-config.service';

@Component({
    selector: 'artifact-summary',
    templateUrl: './artifact-summary.component.html',
    styleUrls: ['./artifact-summary.component.scss'],

    providers: [],
})
export class ArtifactSummaryComponent implements OnInit {
    tagId: string;
    artifactDigest: string;
    sbomDigest?: string;
    activeTab?: string;
    repositoryName: string;
    projectId: string | number;
    referArtifactNameArray: string[] = [];

    labels: Label;
    artifact: Artifact;
    @Output()
    backEvt: EventEmitter<any> = new EventEmitter<any>();
    projectName: string;
    isProxyCacheProject: boolean = false;
    loading: boolean = false;
    registryUrl: string;

    constructor(
        private route: ActivatedRoute,
        private router: Router,
        private frontEndArtifactService: ArtifactService,
        private event: EventService,
        private appConfigService: AppConfigService
    ) {}

    goBack(): void {
        this.router.navigate([
            'harbor',
            'projects',
            this.projectId,
            'repositories',
            this.repositoryName,
        ]);
    }

    goBackRep(): void {
        this.router.navigate([
            'harbor',
            'projects',
            this.projectId,
            'repositories',
        ]);
    }

    goBackPro(): void {
        this.router.navigate(['harbor', 'projects']);
    }
    jumpDigest(index: number) {
        const arr: string[] = this.referArtifactNameArray.slice(0, index + 1);
        if (arr && arr.length) {
            this.router.navigate([
                'harbor',
                'projects',
                this.projectId,
                'repositories',
                this.repositoryName,
                'artifacts-tab',
                'depth',
                arr.join('-'),
            ]);
        } else {
            this.router.navigate([
                'harbor',
                'projects',
                this.projectId,
                'repositories',
                this.repositoryName,
            ]);
        }
    }

    ngOnInit(): void {
        this.registryUrl = this.appConfigService.getConfig()?.registry_url;
        let depth = this.route.snapshot.params['depth'];
        if (depth) {
            this.referArtifactNameArray = depth.split('-');
        }
        this.repositoryName = this.route.snapshot.params['repo'];
        this.artifactDigest = this.route.snapshot.params['digest'];
        this.projectId = this.route.snapshot.parent.params['id'];
        this.sbomDigest = this.route.snapshot.queryParams['sbomDigest'];
        this.activeTab = this.route.snapshot.queryParams['tab'];
        if (this.repositoryName && this.artifactDigest) {
            const resolverData = this.route.snapshot.data;
            if (resolverData) {
                const pro: Project = <Project>(
                    resolverData['artifactResolver'][1]
                );
                this.projectName = pro.name;
                if (pro.registry_id) {
                    this.isProxyCacheProject = true;
                }
                this.artifact = <Artifact>resolverData['artifactResolver'][0];
                this.getIconFromBackEnd();
            }
        }
        // scroll to the top for harbor container HTML element
        this.event.publish(HarborEvent.SCROLL_TO_POSITION, 0);
    }
    onBack(): void {
        this.backEvt.emit(this.repositoryName);
    }
    showDefaultIcon(event: any) {
        if (event && event.target) {
            event.target.src = this.getFallbackIcon();
        }
    }

    isNpmArtifact(): boolean {
        return isNpmArtifact(this.artifact);
    }

    isMavenArtifact(): boolean {
        return isMavenArtifact(this.artifact);
    }

    isHomebrewArtifact(): boolean {
        return isHomebrewArtifact(this.artifact);
    }

    isPypiArtifact(): boolean {
        return isPypiArtifact(this.artifact);
    }

    isCargoArtifact(): boolean {
        return isCargoArtifact(this.artifact);
    }

    isBootcArtifact(): boolean {
        return isBootcArtifact(this.artifact);
    }

    hasArtifactIcon(): boolean {
        return (
            this.isNpmArtifact() ||
            this.isMavenArtifact() ||
            this.isHomebrewArtifact() ||
            this.isPypiArtifact() ||
            this.isCargoArtifact() ||
            this.isBootcArtifact() ||
            !!this.artifact?.icon
        );
    }

    getArtifactIcon(): SafeUrl | string {
        if (this.isNpmArtifact()) {
            return artifactNpm;
        }
        if (this.isMavenArtifact()) {
            return artifactMaven;
        }
        if (this.isHomebrewArtifact()) {
            return artifactHomebrew;
        }
        if (this.isPypiArtifact()) {
            return artifactPypi;
        }
        if (this.isCargoArtifact()) {
            return artifactCargo;
        }
        if (this.isBootcArtifact()) {
            return artifactBootc;
        }
        return this.getIcon(this.artifact.icon);
    }

    getFallbackIcon(): string {
        if (this.isNpmArtifact()) {
            return artifactNpm;
        }
        if (this.isMavenArtifact()) {
            return artifactMaven;
        }
        if (this.isHomebrewArtifact()) {
            return artifactHomebrew;
        }
        if (this.isPypiArtifact()) {
            return artifactPypi;
        }
        if (this.isCargoArtifact()) {
            return artifactCargo;
        }
        if (this.isBootcArtifact()) {
            return artifactBootc;
        }
        return artifactDefault;
    }

    getIcon(icon: string): SafeUrl {
        return this.frontEndArtifactService.getIcon(icon);
    }
    isYanked(): boolean {
        return this.artifact?.extra_attrs?.['yanked'] === true;
    }
    getIconFromBackEnd() {
        if (this.artifact && this.artifact.icon) {
            this.frontEndArtifactService.getIconsFromBackEnd([this.artifact]);
        }
    }
}
