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

package event

import (
	"fmt"
	"time"

	"github.com/goharbor/harbor/src/common/rbac"
	"github.com/goharbor/harbor/src/lib/selector"
	"github.com/goharbor/harbor/src/pkg/artifact"
	"github.com/goharbor/harbor/src/pkg/auditext/desc"
	"github.com/goharbor/harbor/src/pkg/auditext/model"
	proModels "github.com/goharbor/harbor/src/pkg/project/models"
	robotModel "github.com/goharbor/harbor/src/pkg/robot/model"
	v1 "github.com/goharbor/harbor/src/pkg/scan/rest/v1"
)

// the event consumers can refer to this file to find all topics and the corresponding event structures

// const definition
const (
	TopicCreateProject     = "CREATE_PROJECT"
	TopicDeleteProject     = "DELETE_PROJECT"
	TopicPushArtifact      = "PUSH_ARTIFACT"
	TopicPullArtifact      = "PULL_ARTIFACT"
	TopicDeleteArtifact    = "DELETE_ARTIFACT"
	TopicDeleteRepository  = "DELETE_REPOSITORY"
	TopicCreateTag         = "CREATE_TAG"
	TopicDeleteTag         = "DELETE_TAG"
	TopicScanningFailed    = "SCANNING_FAILED"
	TopicScanningStopped   = "SCANNING_STOPPED"
	TopicScanningCompleted = "SCANNING_COMPLETED"
	// QuotaExceedTopic is topic for quota warning event, the usage reaches the warning bar of limitation, like 85%
	TopicQuotaWarning      = "QUOTA_WARNING"
	TopicQuotaExceed       = "QUOTA_EXCEED"
	TopicReplication       = "REPLICATION"
	TopicArtifactLabeled   = "ARTIFACT_LABELED"
	TopicTagRetention      = "TAG_RETENTION"
	TopicCreateRobot       = "CREATE_ROBOT"
	TopicDeleteRobot       = "DELETE_ROBOT"
	TopicCommonEvent       = "COMMON_API"
	ResourceTypeProject    = "project"
	ResourceTypeArtifact   = "artifact"
	ResourceTypeRepository = "repository"
	ResourceTypeRobot      = "robot"
	ResourceTypeTag        = "tag"
)

// CreateProjectEvent is the creating project event
type CreateProjectEvent struct {
	EventType string
	ProjectID int64
	Project   string
	Operator  string
	OccurAt   time.Time
}

// ResolveToAuditLog ...
func (c *CreateProjectEvent) ResolveToAuditLog() (*model.AuditLogExt, error) {
	auditLog := &model.AuditLogExt{
		ProjectID:            c.ProjectID,
		OpTime:               c.OccurAt,
		Operation:            rbac.ActionCreate.String(),
		Username:             c.Operator,
		ResourceType:         ResourceTypeProject,
		IsSuccessful:         true,
		OperationDescription: desc.New(c.Operator, "create", ResourceTypeProject).Name(c.Project).Set("id", c.ProjectID).String(),
		Resource:             c.Project}
	return auditLog, nil
}

func (c *CreateProjectEvent) String() string {
	return fmt.Sprintf("ID-%d Name-%s Operator-%s OccurAt-%s",
		c.ProjectID, c.Project, c.Operator, c.OccurAt.Format("2006-01-02 15:04:05"))
}

// DeleteProjectEvent is the deleting project event
type DeleteProjectEvent struct {
	EventType string
	ProjectID int64
	Project   string
	Operator  string
	OccurAt   time.Time
}

// ResolveToAuditLog ...
func (d *DeleteProjectEvent) ResolveToAuditLog() (*model.AuditLogExt, error) {
	auditLog := &model.AuditLogExt{
		ProjectID:            d.ProjectID,
		OpTime:               d.OccurAt,
		Operation:            rbac.ActionDelete.String(),
		Username:             d.Operator,
		ResourceType:         ResourceTypeProject,
		IsSuccessful:         true,
		OperationDescription: desc.New(d.Operator, "delete", ResourceTypeProject).Name(d.Project).Set("id", d.ProjectID).String(),
		Resource:             d.Project}
	return auditLog, nil
}

func (d *DeleteProjectEvent) String() string {
	return fmt.Sprintf("ID-%d Name-%s Operator-%s OccurAt-%s",
		d.ProjectID, d.Project, d.Operator, d.OccurAt.Format("2006-01-02 15:04:05"))
}

// DeleteRepositoryEvent is the deleting repository event
type DeleteRepositoryEvent struct {
	EventType  string
	ProjectID  int64
	Repository string
	Operator   string
	OccurAt    time.Time
}

// ResolveToAuditLog ...
func (d *DeleteRepositoryEvent) ResolveToAuditLog() (*model.AuditLogExt, error) {
	auditLog := &model.AuditLogExt{
		ProjectID:            d.ProjectID,
		OpTime:               d.OccurAt,
		Operation:            rbac.ActionDelete.String(),
		Username:             d.Operator,
		ResourceType:         ResourceTypeRepository,
		IsSuccessful:         true,
		OperationDescription: desc.New(d.Operator, "delete", ResourceTypeRepository).Name(d.Repository).String(),
		Resource:             d.Repository,
	}
	return auditLog, nil
}

func (d *DeleteRepositoryEvent) String() string {
	return fmt.Sprintf("ID-%d Repository-%s Operator-%s OccurAt-%s",
		d.ProjectID, d.Repository, d.Operator, d.OccurAt.Format("2006-01-02 15:04:05"))
}

// ArtifactEvent is the pushing/pulling artifact event
type ArtifactEvent struct {
	EventType  string
	Repository string
	Artifact   *artifact.Artifact
	Tags       []string // when the artifact is pushed by digest, the tag here will be null
	Labels     []string
	Operator   string
	OccurAt    time.Time
	// ClientAddress is the source IP address of the request that triggered
	// the event. Empty when the event originates outside an HTTP request
	// (e.g. replication, retention).
	ClientAddress string
	// UserAgent is the User-Agent header of the request that triggered the
	// event.
	UserAgent string
}

func (a *ArtifactEvent) String() string {
	return fmt.Sprintf("ID-%d, Repository-%s Tags-%s Digest-%s Operator-%s OccurAt-%s",
		a.Artifact.ID, a.Repository, a.Tags, a.Artifact.Digest, a.Operator,
		a.OccurAt.Format("2006-01-02 15:04:05"))
}

// resource names the artifact by its first tag, or by digest when untagged
func (a *ArtifactEvent) resource() string {
	if len(a.Tags) == 0 {
		return fmt.Sprintf("%s@%s", a.Artifact.RepositoryName, a.Artifact.Digest)
	}
	return fmt.Sprintf("%s:%s", a.Artifact.RepositoryName, a.Tags[0])
}

// PushArtifactEvent is the pushing artifact event
type PushArtifactEvent struct {
	*ArtifactEvent
}

// ResolveToAuditLog ...
func (p *PushArtifactEvent) ResolveToAuditLog() (*model.AuditLogExt, error) {
	resource := p.ArtifactEvent.resource()
	auditLog := &model.AuditLogExt{
		ProjectID:            p.Artifact.ProjectID,
		OpTime:               p.OccurAt,
		Operation:            rbac.ActionCreate.String(),
		Username:             p.Operator,
		IsSuccessful:         true,
		OperationDescription: desc.New(p.Operator, "push", ResourceTypeArtifact).Name(resource).Set("digest", p.Artifact.Digest).Set("type", p.Artifact.Type).Set("size", p.Artifact.Size).String(),
		ResourceType:         ResourceTypeArtifact,
		Resource:             resource,
		ClientAddress:        p.ClientAddress,
		UserAgent:            p.UserAgent}

	return auditLog, nil
}

func (p *PushArtifactEvent) String() string {
	return p.ArtifactEvent.String()
}

// PullArtifactEvent is the pulling artifact event
type PullArtifactEvent struct {
	*ArtifactEvent
}

// ResolveToAuditLog ...
func (p *PullArtifactEvent) ResolveToAuditLog() (*model.AuditLogExt, error) {
	resource := p.ArtifactEvent.resource()
	// for pull public resource
	username := p.Operator
	if username == "" {
		username = "anonymous"
	}
	auditLog := &model.AuditLogExt{
		ProjectID:            p.Artifact.ProjectID,
		OpTime:               p.OccurAt,
		Operation:            rbac.ActionPull.String(),
		Username:             username,
		IsSuccessful:         true,
		OperationDescription: desc.New(username, "pull", ResourceTypeArtifact).Name(resource).Set("digest", p.Artifact.Digest).String(),
		ResourceType:         ResourceTypeArtifact,
		Resource:             resource,
		ClientAddress:        p.ClientAddress,
		UserAgent:            p.UserAgent}

	return auditLog, nil
}

func (p *PullArtifactEvent) String() string {
	return p.ArtifactEvent.String()
}

// DeleteArtifactEvent is the deleting artifact event
type DeleteArtifactEvent struct {
	*ArtifactEvent
}

// ResolveToAuditLog ...
func (d *DeleteArtifactEvent) ResolveToAuditLog() (*model.AuditLogExt, error) {
	auditLog := &model.AuditLogExt{
		ProjectID:            d.Artifact.ProjectID,
		OpTime:               d.OccurAt,
		Operation:            rbac.ActionDelete.String(),
		Username:             d.Operator,
		ResourceType:         ResourceTypeArtifact,
		IsSuccessful:         true,
		OperationDescription: desc.New(d.Operator, "delete", ResourceTypeArtifact).Name(d.Artifact.RepositoryName+"@"+d.Artifact.Digest).Set("tags", d.Tags).Set("labels", d.Labels).Set("size", d.Artifact.Size).String(),
		Resource:             fmt.Sprintf("%s@%s", d.Artifact.RepositoryName, d.Artifact.Digest),
		ClientAddress:        d.ClientAddress,
		UserAgent:            d.UserAgent}
	return auditLog, nil
}

func (d *DeleteArtifactEvent) String() string {
	return d.ArtifactEvent.String()
}

// CreateTagEvent is the creating tag event
type CreateTagEvent struct {
	EventType        string
	Repository       string
	Tag              string
	Labels           []string
	AttachedArtifact *artifact.Artifact
	Operator         string
	OccurAt          time.Time
}

// ResolveToAuditLog ...
func (c *CreateTagEvent) ResolveToAuditLog() (*model.AuditLogExt, error) {
	auditLog := &model.AuditLogExt{
		ProjectID:            c.AttachedArtifact.ProjectID,
		OpTime:               c.OccurAt,
		Operation:            rbac.ActionCreate.String(),
		Username:             c.Operator,
		ResourceType:         ResourceTypeTag,
		IsSuccessful:         true,
		OperationDescription: desc.New(c.Operator, "create", ResourceTypeTag).Name(c.Repository+":"+c.Tag).Set("digest", c.AttachedArtifact.Digest).String(),
		Resource:             fmt.Sprintf("%s:%s", c.Repository, c.Tag)}
	return auditLog, nil
}

func (c *CreateTagEvent) String() string {
	return fmt.Sprintf("ArtifactID-%d, Repository-%s Tag-%s Digest-%s Operator-%s OccurAt-%s",
		c.AttachedArtifact.ID, c.Repository, c.Tag, c.AttachedArtifact.Digest, c.Operator,
		c.OccurAt.Format("2006-01-02 15:04:05"))
}

// DeleteTagEvent is the deleting tag event
type DeleteTagEvent struct {
	EventType        string
	Repository       string
	Tag              string
	Labels           []string
	AttachedArtifact *artifact.Artifact
	Operator         string
	OccurAt          time.Time
}

// ResolveToAuditLog ...
func (d *DeleteTagEvent) ResolveToAuditLog() (*model.AuditLogExt, error) {
	auditLog := &model.AuditLogExt{
		ProjectID:            d.AttachedArtifact.ProjectID,
		OpTime:               d.OccurAt,
		Operation:            rbac.ActionDelete.String(),
		Username:             d.Operator,
		ResourceType:         ResourceTypeTag,
		IsSuccessful:         true,
		OperationDescription: desc.New(d.Operator, "delete", ResourceTypeTag).Name(d.Repository+":"+d.Tag).Set("digest", d.AttachedArtifact.Digest).String(),
		Resource:             fmt.Sprintf("%s:%s", d.Repository, d.Tag)}
	return auditLog, nil
}

func (d *DeleteTagEvent) String() string {
	return fmt.Sprintf("ArtifactID-%d, Repository-%s Tag-%s Digest-%s Operator-%s OccurAt-%s",
		d.AttachedArtifact.ID, d.Repository, d.Tag, d.AttachedArtifact.Digest, d.Operator,
		d.OccurAt.Format("2006-01-02 15:04:05"))
}

// ScanImageEvent is scanning image related event data to publish
type ScanImageEvent struct {
	EventType string
	ScanType  string
	Artifact  *v1.Artifact
	OccurAt   time.Time
	Operator  string
}

func (s *ScanImageEvent) String() string {
	return fmt.Sprintf("Artifact-%+v Operator-%s OccurAt-%s",
		s.Artifact, s.Operator, s.OccurAt.Format("2006-01-02 15:04:05"))
}

// QuotaEvent is project quota related event data to publish
type QuotaEvent struct {
	EventType string
	Project   *proModels.Project
	Resource  *ImgResource
	OccurAt   time.Time
	RepoName  string
	Msg       string
	Operator  string
}

func (q *QuotaEvent) String() string {
	return fmt.Sprintf("ProjectID-%d RepoName-%s Resource-%+v Msg-%s OccurAt-%s",
		q.Project.ProjectID, q.RepoName, q.Resource, q.Msg, q.OccurAt.Format("2006-01-02 15:04:05"))
}

// ImgResource include image digest and tag
type ImgResource struct {
	Digest string
	Tag    string
}

// ReplicationEvent is replication related event data to publish
type ReplicationEvent struct {
	EventType         string
	ReplicationTaskID int64
	OccurAt           time.Time
	Status            string
}

func (r *ReplicationEvent) String() string {
	return fmt.Sprintf("ReplicationTaskID-%d Status-%s OccurAt-%s",
		r.ReplicationTaskID, r.Status, r.OccurAt.Format("2006-01-02 15:04:05"))
}

// ArtifactLabeledEvent is event data of artifact labeled
type ArtifactLabeledEvent struct {
	ArtifactID int64
	LabelID    int64
	OccurAt    time.Time
	Operator   string
}

func (al *ArtifactLabeledEvent) String() string {
	return fmt.Sprintf("ArtifactID-%d LabelID-%d Operator-%s OccurAt-%s",
		al.ArtifactID, al.LabelID, al.Operator, al.OccurAt.Format("2006-01-02 15:04:05"))
}

// RetentionEvent is tag retention related event data to publish
type RetentionEvent struct {
	TaskID    int64
	EventType string
	OccurAt   time.Time
	Status    string
	Deleted   []*selector.Result
	Total     int
	Retained  int
}

func (r *RetentionEvent) String() string {
	candidates := []string{}
	for _, candidate := range r.Deleted {
		candidates = append(candidates, fmt.Sprintf("%s:%s:%s", candidate.Target.Namespace,
			candidate.Target.Repository, candidate.Target.Tags))
	}

	return fmt.Sprintf("TaskID-%d Status-%s Deleted-%s OccurAt-%s",
		r.TaskID, r.Status, candidates, r.OccurAt.Format("2006-01-02 15:04:05"))
}

// CreateRobotEvent is the creating robot event
type CreateRobotEvent struct {
	EventType string
	Robot     *robotModel.Robot
	Operator  string
	OccurAt   time.Time
}

// ResolveToAuditLog ...
func (c *CreateRobotEvent) ResolveToAuditLog() (*model.AuditLogExt, error) {
	auditLog := &model.AuditLogExt{
		ProjectID:            c.Robot.ProjectID,
		OpTime:               c.OccurAt,
		Operation:            rbac.ActionCreate.String(),
		Username:             c.Operator,
		ResourceType:         ResourceTypeRobot,
		IsSuccessful:         true,
		OperationDescription: desc.New(c.Operator, "create", ResourceTypeRobot).Name(c.Robot.Name).Set("id", c.Robot.ID).Set("level", robotLevel(c.Robot)).String(),
		Resource:             c.Robot.Name}
	return auditLog, nil
}

func (c *CreateRobotEvent) String() string {
	return fmt.Sprintf("Name-%s Operator-%s OccurAt-%s",
		c.Robot.Name, c.Operator, c.OccurAt.Format("2006-01-02 15:04:05"))
}

// robotLevel names the scope of a robot the way the API does
func robotLevel(r *robotModel.Robot) string {
	if r.ProjectID == 0 {
		return "system"
	}
	return "project"
}

// DeleteRobotEvent is the deleting robot event
type DeleteRobotEvent struct {
	EventType string
	Robot     *robotModel.Robot
	Operator  string
	OccurAt   time.Time
}

// ResolveToAuditLog ...
func (c *DeleteRobotEvent) ResolveToAuditLog() (*model.AuditLogExt, error) {
	auditLog := &model.AuditLogExt{
		ProjectID:            c.Robot.ProjectID,
		OpTime:               c.OccurAt,
		Operation:            rbac.ActionDelete.String(),
		Username:             c.Operator,
		ResourceType:         ResourceTypeRobot,
		IsSuccessful:         true,
		OperationDescription: desc.New(c.Operator, "delete", ResourceTypeRobot).Name(c.Robot.Name).Set("id", c.Robot.ID).Set("level", robotLevel(c.Robot)).String(),
		Resource:             c.Robot.Name}
	return auditLog, nil
}

func (c *DeleteRobotEvent) String() string {
	return fmt.Sprintf("Name-%s Operator-%s OccurAt-%s",
		c.Robot.Name, c.Operator, c.OccurAt.Format("2006-01-02 15:04:05"))
}
