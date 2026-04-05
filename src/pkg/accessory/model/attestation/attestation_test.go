package attestation

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/pkg/accessory/model"
	htesting "github.com/goharbor/harbor/src/testing"
)

type AttestationTestSuite struct {
	htesting.Suite
	accessory model.Accessory
	digest    string
	subDigest string
}

func (suite *AttestationTestSuite) SetupSuite() {
	suite.digest = suite.DigestString()
	suite.subDigest = suite.DigestString()
	suite.accessory, _ = model.New(model.TypeBuildKitAttestation, model.AccessoryData{
		ArtifactID:        1,
		SubArtifactDigest: suite.subDigest,
		Size:              1234,
		Digest:            suite.digest,
	})
}

func (suite *AttestationTestSuite) TestGetArtID() {
	suite.Equal(int64(1), suite.accessory.GetData().ArtifactID)
}

func (suite *AttestationTestSuite) TestGetDigest() {
	suite.Equal(suite.digest, suite.accessory.GetData().Digest)
}

func (suite *AttestationTestSuite) TestGetType() {
	suite.Equal(model.TypeBuildKitAttestation, suite.accessory.GetData().Type)
}

func (suite *AttestationTestSuite) TestKind() {
	suite.Equal(model.RefHard, suite.accessory.Kind())
}

func (suite *AttestationTestSuite) TestIsHard() {
	suite.True(suite.accessory.IsHard())
	suite.False(suite.accessory.IsSoft())
}

func (suite *AttestationTestSuite) TestDisplay() {
	suite.False(suite.accessory.Display())
}

func TestAttestationTestSuite(t *testing.T) {
	suite.Run(t, new(AttestationTestSuite))
}
