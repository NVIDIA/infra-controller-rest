package operatingsystem

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	osActivity "github.com/NVIDIA/ncx-infra-controller-rest/workflow/pkg/activity/operatingsystem"
)

type DeleteOperatingSystemByIDTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env *testsuite.TestWorkflowEnvironment
}

func (s *DeleteOperatingSystemByIDTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *DeleteOperatingSystemByIDTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

func (s *DeleteOperatingSystemByIDTestSuite) Test_DeleteOperatingSystemByID_Success() {
	var osManager osActivity.ManageOperatingSystem

	siteID := uuid.New()

	operatingSystemID := uuid.New()

	// Mock DeleteOperatingSystemViaSiteAgent activity
	s.env.RegisterActivity(osManager.DeleteOperatingSystemViaSiteAgent)
	s.env.OnActivity(osManager.DeleteOperatingSystemViaSiteAgent, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	// execute DeleteOperatingSystem workflow
	s.env.ExecuteWorkflow(DeleteOperatingSystemByID, []uuid.UUID{siteID}, operatingSystemID)
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

func (s *DeleteOperatingSystemByIDTestSuite) Test_DeleteOperatingSystemByID_ActivityFails() {
	var osManager osActivity.ManageOperatingSystem

	siteID := uuid.New()

	operatingSystemID := uuid.New()

	// Mock DeleteOperatingSystemViaSiteAgent activity failure
	s.env.RegisterActivity(osManager.DeleteOperatingSystemViaSiteAgent)
	s.env.OnActivity(osManager.DeleteOperatingSystemViaSiteAgent, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("DeleteOperatingSystemViaSiteAgent Failure"))

	// execute DeleteOperatingSystemByID workflow
	s.env.ExecuteWorkflow(DeleteOperatingSystemByID, []uuid.UUID{siteID}, operatingSystemID)
	s.True(s.env.IsWorkflowCompleted())
	err := s.env.GetWorkflowError()
	s.Error(err)

	var applicationErr *temporal.ApplicationError
	s.True(errors.As(err, &applicationErr))
	s.Equal("DeleteOperatingSystemViaSiteAgent Failure", applicationErr.Error())
}

func TestDeleteOperatingSystemByIDSuite(t *testing.T) {
	suite.Run(t, new(DeleteOperatingSystemByIDTestSuite))
}
