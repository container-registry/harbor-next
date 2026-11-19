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
export interface PushStep {
    labelKey: string;
    command: string;
}

// A labelled group of sections, e.g. every OCI client.
export interface PushGroup {
    id: string;
    titleKey: string;
    // i18n key of a label shown next to the group title.
    badgeKey?: string;
}

// One client's push flow: the steps a user copies, in order.
export interface PushSection {
    id: string;
    titleKey: string;
    // Heading the section is listed under, in the dialog and the guide.
    group: PushGroup;
    steps: PushStep[];
    // i18n key of a note shown under the steps.
    hintKey?: string;
    // Feature key the section needs; always shown when unset.
    feature?: string;
}

// A group with its sections, in list order.
export interface PushSectionGroup extends PushGroup {
    sections: PushSection[];
}

export type PushSectionSource = (
    registryUrl: string,
    project: string
) => PushSection[];

export const OCI_PUSH_GROUP: PushGroup = {
    id: 'oci',
    titleKey: 'PUSH_IMAGE.GROUP_OCI',
};

// Push commands for the OCI clients, one section per client.
export function containerPushSections(
    registryUrl: string,
    project: string
): PushSection[] {
    const target = `${registryUrl}/${project}`;
    return [
        {
            id: 'docker',
            titleKey: 'PUSH_IMAGE.DOCKER',
            group: OCI_PUSH_GROUP,
            steps: [
                {
                    labelKey: 'PUSH_IMAGE.TAG_COMMAND',
                    command: `docker tag SOURCE_IMAGE[:TAG] ${target}/REPOSITORY[:TAG]`,
                },
                {
                    labelKey: 'PUSH_IMAGE.PUSH_COMMAND',
                    command: `docker push ${target}/REPOSITORY[:TAG]`,
                },
            ],
        },
        {
            id: 'podman',
            titleKey: 'PUSH_IMAGE.PODMAN',
            group: OCI_PUSH_GROUP,
            steps: [
                {
                    labelKey: 'PUSH_IMAGE.PUSH_COMMAND',
                    command: `podman push IMAGE_ID ${target}/REPOSITORY[:TAG]`,
                },
            ],
        },
        {
            id: 'helm',
            titleKey: 'PUSH_IMAGE.HELM',
            group: OCI_PUSH_GROUP,
            steps: [
                {
                    labelKey: 'PUSH_IMAGE.TAG_COMMAND_CHART',
                    command: `helm package CHART_PATH`,
                },
                {
                    labelKey: 'PUSH_IMAGE.PUSH_COMMAND_CHART',
                    command: `helm push CHART_PACKAGE oci://${target}`,
                },
            ],
        },
        {
            id: 'cnab',
            titleKey: 'PUSH_IMAGE.CNAB',
            group: OCI_PUSH_GROUP,
            steps: [
                {
                    labelKey: 'PUSH_IMAGE.PUSH_COMMAND_CNAB',
                    command: `cnab-to-oci push CNAB_PATH --target ${target}/REPOSITORY[:TAG] --auto-update-bundle`,
                },
            ],
        },
    ];
}

// Everything the push dialog offers, in tab order.
export const PUSH_SECTION_SOURCES: PushSectionSource[] = [
    containerPushSections,
];

// The enabled sections of every source, in tab order.
export function pushSections(
    registryUrl: string,
    project: string,
    featureEnabled: (key: string) => boolean
): PushSection[] {
    return PUSH_SECTION_SOURCES.reduce<PushSection[]>(
        (all, source) => all.concat(source(registryUrl, project)),
        []
    ).filter(section => !section.feature || featureEnabled(section.feature));
}

// Collects sections under their group; groups keep the order they first appear in.
export function groupPushSections(sections: PushSection[]): PushSectionGroup[] {
    const groups: PushSectionGroup[] = [];
    for (const section of sections) {
        let group = groups.find(g => g.id === section.group.id);
        if (!group) {
            group = { ...section.group, sections: [] };
            groups.push(group);
        }
        group.sections.push(section);
    }
    return groups;
}
