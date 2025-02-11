package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/openshift/assisted-service/internal/host"
	"github.com/openshift/assisted-service/internal/metrics"
	"github.com/sirupsen/logrus"

	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/openshift/assisted-service/internal/common"
	"github.com/openshift/assisted-service/internal/events"
	"github.com/openshift/assisted-service/models"
)

var _ = Describe("Transition tests", func() {
	var (
		ctx           = context.Background()
		capi          API
		db            *gorm.DB
		clusterId     strfmt.UUID
		eventsHandler events.Handler
		ctrl          *gomock.Controller
		mockMetric    *metrics.MockAPI
		dbName        = "cluster_transition_test"
	)

	BeforeEach(func() {
		db = common.PrepareTestDB(dbName, &events.Event{})
		eventsHandler = events.New(db, logrus.New())
		ctrl = gomock.NewController(GinkgoT())
		mockMetric = metrics.NewMockAPI(ctrl)
		capi = NewManager(getDefaultConfig(), getTestLog(), db, eventsHandler, nil, mockMetric, nil)
		clusterId = strfmt.UUID(uuid.New().String())
	})

	Context("cancel_installation", func() {
		It("cancel_installation", func() {
			c := common.Cluster{
				Cluster: models.Cluster{ID: &clusterId, Status: swag.String(models.ClusterStatusInstalling)},
			}
			Expect(db.Create(&c).Error).ShouldNot(HaveOccurred())
			mockMetric.EXPECT().ClusterInstallationFinished(gomock.Any(), "canceled", c.OpenshiftVersion, *c.ID, c.InstallStartedAt)
			Expect(capi.CancelInstallation(ctx, &c, "", db)).ShouldNot(HaveOccurred())

			Expect(db.First(&c, "id = ?", c.ID).Error).ShouldNot(HaveOccurred())
			Expect(swag.StringValue(c.Status)).Should(Equal(models.ClusterStatusCancelled))
		})

		It("cancel_installation_conflict", func() {
			c := common.Cluster{
				Cluster: models.Cluster{ID: &clusterId, Status: swag.String(models.ClusterStatusInsufficient)},
			}
			Expect(db.Create(&c).Error).ShouldNot(HaveOccurred())
			mockMetric.EXPECT().ClusterInstallationFinished(gomock.Any(), "canceled", c.OpenshiftVersion, *c.ID, c.InstallStartedAt)
			replay := capi.CancelInstallation(ctx, &c, "", db)
			Expect(replay).Should(HaveOccurred())
			Expect(int(replay.StatusCode())).Should(Equal(http.StatusConflict))

			Expect(db.First(&c, "id = ?", c.ID).Error).ShouldNot(HaveOccurred())
			Expect(swag.StringValue(c.Status)).Should(Equal(models.ClusterStatusInsufficient))
		})

		It("cancel_failed_installation", func() {
			c := common.Cluster{
				Cluster: models.Cluster{
					ID:         &clusterId,
					StatusInfo: swag.String("original error"),
					Status:     swag.String(models.ClusterStatusError)},
			}
			Expect(db.Create(&c).Error).ShouldNot(HaveOccurred())
			mockMetric.EXPECT().ClusterInstallationFinished(gomock.Any(), "canceled", c.OpenshiftVersion, *c.ID, c.InstallStartedAt)
			Expect(capi.CancelInstallation(ctx, &c, "", db)).ShouldNot(HaveOccurred())

			Expect(db.First(&c, "id = ?", c.ID).Error).ShouldNot(HaveOccurred())
			Expect(swag.StringValue(c.Status)).Should(Equal(models.ClusterStatusCancelled))
			Expect(swag.StringValue(c.StatusInfo)).ShouldNot(Equal("original error"))
		})
	})
	Context("complete_installation", func() {
		It("complete installation success", func() {
			c := common.Cluster{
				Cluster: models.Cluster{ID: &clusterId, Status: swag.String(models.ClusterStatusFinalizing)},
			}
			Expect(db.Create(&c).Error).ShouldNot(HaveOccurred())
			mockMetric.EXPECT().ClusterInstallationFinished(gomock.Any(), models.ClusterStatusInstalled, c.OpenshiftVersion, *c.ID, c.InstallStartedAt)
			Expect(capi.CompleteInstallation(ctx, &c, true, models.ClusterStatusInstalled)).ShouldNot(HaveOccurred())

			Expect(db.First(&c, "id = ?", c.ID).Error).ShouldNot(HaveOccurred())
			Expect(swag.StringValue(c.Status)).Should(Equal(models.ClusterStatusInstalled))
		})

		It("complete installation failed", func() {
			c := common.Cluster{
				Cluster: models.Cluster{ID: &clusterId, Status: swag.String(models.ClusterStatusFinalizing)},
			}
			Expect(db.Create(&c).Error).ShouldNot(HaveOccurred())
			mockMetric.EXPECT().ClusterInstallationFinished(gomock.Any(), models.ClusterStatusError, c.OpenshiftVersion, *c.ID, c.InstallStartedAt)
			Expect(capi.CompleteInstallation(ctx, &c, false, "aaaa")).ShouldNot(HaveOccurred())

			Expect(db.First(&c, "id = ?", c.ID).Error).ShouldNot(HaveOccurred())
			Expect(swag.StringValue(c.Status)).Should(Equal(models.ClusterStatusError))
			Expect(swag.StringValue(c.StatusInfo)).Should(Equal("aaaa"))

		})

		It("complete_installation_conflict", func() {
			c := common.Cluster{
				Cluster: models.Cluster{ID: &clusterId, Status: swag.String(models.ClusterStatusInstalling)},
			}
			Expect(db.Create(&c).Error).ShouldNot(HaveOccurred())
			mockMetric.EXPECT().ClusterInstallationFinished(gomock.Any(), models.ClusterStatusInstalled, c.OpenshiftVersion, *c.ID, c.InstallStartedAt)
			replay := capi.CompleteInstallation(ctx, &c, true, "")
			Expect(replay).Should(HaveOccurred())
			Expect(int(replay.StatusCode())).Should(Equal(http.StatusConflict))

			Expect(db.First(&c, "id = ?", c.ID).Error).ShouldNot(HaveOccurred())
			Expect(swag.StringValue(c.Status)).Should(Equal(models.ClusterStatusInstalling))
		})

		It("complete_installation_conflict_failed", func() {
			c := common.Cluster{
				Cluster: models.Cluster{ID: &clusterId, Status: swag.String(models.ClusterStatusInstalling)},
			}
			Expect(db.Create(&c).Error).ShouldNot(HaveOccurred())
			mockMetric.EXPECT().ClusterInstallationFinished(gomock.Any(), models.ClusterStatusError, c.OpenshiftVersion, *c.ID, c.InstallStartedAt)
			replay := capi.CompleteInstallation(ctx, &c, false, "")
			Expect(replay).Should(HaveOccurred())
			Expect(int(replay.StatusCode())).Should(Equal(http.StatusConflict))

			Expect(db.First(&c, "id = ?", c.ID).Error).ShouldNot(HaveOccurred())
			Expect(swag.StringValue(c.Status)).Should(Equal(models.ClusterStatusInstalling))
		})
	})
	AfterEach(func() {
		common.DeleteTestDB(db, dbName)
	})
})

var _ = Describe("Cancel cluster installation", func() {
	var (
		ctx               = context.Background()
		dbName            = "cancel_cluster_installation_test"
		capi              API
		db                *gorm.DB
		ctrl              *gomock.Controller
		mockEventsHandler *events.MockHandler
		mockMetric        *metrics.MockAPI
	)

	BeforeEach(func() {
		db = common.PrepareTestDB(dbName, &events.Event{})
		ctrl = gomock.NewController(GinkgoT())
		mockEventsHandler = events.NewMockHandler(ctrl)
		mockMetric = metrics.NewMockAPI(ctrl)
		capi = NewManager(getDefaultConfig(), getTestLog(), db, mockEventsHandler, nil, mockMetric, nil)
	})

	acceptNewEvents := func(times int) {
		mockEventsHandler.EXPECT().AddEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(times)
	}

	acceptClusterInstallationFinished := func(times int) {
		mockMetric.EXPECT().ClusterInstallationFinished(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(times)
	}

	tests := []struct {
		state      string
		success    bool
		statusCode int32
	}{
		{state: models.ClusterStatusPreparingForInstallation, success: true},
		{state: models.ClusterStatusInstalling, success: true},
		{state: models.ClusterStatusError, success: true},
		{state: models.ClusterStatusInsufficient, success: false, statusCode: http.StatusConflict},
		{state: models.ClusterStatusReady, success: false, statusCode: http.StatusConflict},
		{state: models.ClusterStatusFinalizing, success: false, statusCode: http.StatusConflict},
		{state: models.ClusterStatusInstalled, success: false, statusCode: http.StatusConflict},
	}

	for _, t := range tests {
		It(fmt.Sprintf("cancel from state %s", t.state), func() {
			clusterId := strfmt.UUID(uuid.New().String())
			cluster := common.Cluster{
				Cluster: models.Cluster{ID: &clusterId, Status: swag.String(t.state)},
			}
			Expect(db.Create(&cluster).Error).ShouldNot(HaveOccurred())
			eventsNum := 1
			if t.success {
				eventsNum++
				acceptClusterInstallationFinished(1)
			}
			acceptNewEvents(eventsNum)
			err := capi.CancelInstallation(ctx, &cluster, "reason", db)
			if t.success {
				Expect(err).ShouldNot(HaveOccurred())
			} else {
				Expect(err).Should(HaveOccurred())
				Expect(err.StatusCode()).Should(Equal(t.statusCode))
			}
		})
	}

	AfterEach(func() {
		ctrl.Finish()
		common.DeleteTestDB(db, dbName)
	})
})

var _ = Describe("Reset cluster", func() {
	var (
		ctx               = context.Background()
		dbName            = "reset_cluster_test"
		capi              API
		db                *gorm.DB
		ctrl              *gomock.Controller
		mockEventsHandler *events.MockHandler
	)

	BeforeEach(func() {
		db = common.PrepareTestDB(dbName, &events.Event{})
		ctrl = gomock.NewController(GinkgoT())
		mockEventsHandler = events.NewMockHandler(ctrl)
		capi = NewManager(getDefaultConfig(), getTestLog(), db, mockEventsHandler, nil, nil, nil)
	})

	acceptNewEvents := func(times int) {
		mockEventsHandler.EXPECT().AddEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(times)
	}

	tests := []struct {
		state      string
		success    bool
		statusCode int32
	}{
		{state: models.ClusterStatusPreparingForInstallation, success: true},
		{state: models.ClusterStatusInstalling, success: true},
		{state: models.ClusterStatusError, success: true},
		{state: models.ClusterStatusInsufficient, success: false, statusCode: http.StatusConflict},
		{state: models.ClusterStatusReady, success: false, statusCode: http.StatusConflict},
		{state: models.ClusterStatusFinalizing, success: false, statusCode: http.StatusConflict},
		{state: models.ClusterStatusInstalled, success: false, statusCode: http.StatusConflict},
	}

	for _, t := range tests {
		It(fmt.Sprintf("reset from state %s", t.state), func() {
			clusterId := strfmt.UUID(uuid.New().String())
			cluster := common.Cluster{
				Cluster: models.Cluster{ID: &clusterId, Status: swag.String(t.state)},
			}
			Expect(db.Create(&cluster).Error).ShouldNot(HaveOccurred())
			eventsNum := 1
			if t.success {
				eventsNum++
			}
			acceptNewEvents(eventsNum)
			err := capi.ResetCluster(ctx, &cluster, "reason", db)
			if t.success {
				Expect(err).ShouldNot(HaveOccurred())
			} else {
				Expect(err).Should(HaveOccurred())
				Expect(err.StatusCode()).Should(Equal(t.statusCode))
			}
		})
	}

	AfterEach(func() {
		ctrl.Finish()
		common.DeleteTestDB(db, dbName)
	})
})

type statusInfoChecker interface {
	check(statusInfo *string)
}

type valueChecker struct {
	value string
}

func (v *valueChecker) check(value *string) {
	if value == nil {
		Expect(v.value).To(Equal(""))
	} else {
		Expect(*value).To(Equal(v.value))
	}
}

func makeValueChecker(value string) statusInfoChecker {
	return &valueChecker{value: value}
}

type validationsChecker struct {
	expected map[validationID]validationCheckResult
}

func (j *validationsChecker) check(validationsStr string) {
	validationMap := make(map[string][]validationResult)
	Expect(json.Unmarshal([]byte(validationsStr), &validationMap)).ToNot(HaveOccurred())
next:
	for id, checkedResult := range j.expected {
		category, err := id.category()
		Expect(err).ToNot(HaveOccurred())
		results, ok := validationMap[category]
		Expect(ok).To(BeTrue())
		for _, r := range results {
			if r.ID == id {
				Expect(r.Status).To(Equal(checkedResult.status), "id = %s", id.String())
				Expect(r.Message).To(MatchRegexp(checkedResult.messagePattern))
				continue next
			}
		}
		// Should not reach here
		Expect(false).To(BeTrue())
	}
}

type validationCheckResult struct {
	status         validationStatus
	messagePattern string
}

func makeJsonChecker(expected map[validationID]validationCheckResult) *validationsChecker {
	return &validationsChecker{expected: expected}
}

var _ = Describe("Refresh Cluster - No DHCP", func() {
	var (
		ctx                               = context.Background()
		db                                *gorm.DB
		clusterId, hid1, hid2, hid3, hid4 strfmt.UUID
		cluster                           common.Cluster
		clusterApi                        *Manager
		mockEvents                        *events.MockHandler
		mockHostAPI                       *host.MockAPI
		mockMetric                        *metrics.MockAPI
		ctrl                              *gomock.Controller
		dbName                            string = "cluster_transition_test_refresh_host_no_dhcp"
	)

	mockHostAPIIsRequireUserActionResetFalse := func() {
		mockHostAPI.EXPECT().IsRequireUserActionReset(gomock.Any()).Return(false).AnyTimes()
	}
	BeforeEach(func() {
		db = common.PrepareTestDB(dbName, &events.Event{})
		ctrl = gomock.NewController(GinkgoT())
		mockEvents = events.NewMockHandler(ctrl)
		mockHostAPI = host.NewMockAPI(ctrl)
		mockMetric = metrics.NewMockAPI(ctrl)
		clusterApi = NewManager(getDefaultConfig(), getTestLog().WithField("pkg", "cluster-monitor"), db,
			mockEvents, mockHostAPI, mockMetric, nil)

		hid1 = strfmt.UUID(uuid.New().String())
		hid2 = strfmt.UUID(uuid.New().String())
		hid3 = strfmt.UUID(uuid.New().String())
		hid4 = strfmt.UUID(uuid.New().String())
		clusterId = strfmt.UUID(uuid.New().String())
	})
	Context("All transitions", func() {
		var srcState string
		tests := []struct {
			name               string
			srcState           string
			srcStatusInfo      string
			machineNetworkCidr string
			apiVip             string
			ingressVip         string
			dnsDomain          string
			pullSecretSet      bool
			dstState           string
			hosts              []models.Host
			statusInfoChecker  statusInfoChecker
			validationsChecker *validationsChecker
			errorExpected      bool
		}{
			{
				name:               "pending-for-input to pending-for-input",
				srcState:           models.ClusterStatusPendingForInput,
				dstState:           models.ClusterStatusPendingForInput,
				machineNetworkCidr: "",
				apiVip:             "",
				ingressVip:         "",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Role: models.HostRoleMaster},
				},
				statusInfoChecker: makeValueChecker(statusInfoPendingForInput),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationFailure, messagePattern: "Machine Network CIDR is undefined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationPending, messagePattern: "Machine Network CIDR, API virtual IP, or Ingress virtual IP is undefined"},
					isApiVipDefined:                     {status: ValidationFailure, messagePattern: "API virtual IP is undefined"},
					isApiVipValid:                       {status: ValidationPending, messagePattern: "API virtual IP is undefined"},
					isIngressVipDefined:                 {status: ValidationFailure, messagePattern: "Ingress virtual IP is undefined"},
					isIngressVipValid:                   {status: ValidationPending, messagePattern: "Ingress virtual IP is undefined"},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set."},
					SufficientMastersCount: {status: ValidationFailure,
						messagePattern: fmt.Sprintf("Insufficient number of master host candidates: expected %d",
							common.MinMasterHostsNeededForInstallation)},
				}),
				errorExpected: false,
			},
			{
				name:               "pending-for-input to insufficient - masters > 3",
				srcState:           models.ClusterStatusPendingForInput,
				dstState:           models.ClusterStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
				},
				statusInfoChecker: makeValueChecker(statusInfoInsufficient),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set."},
					SufficientMastersCount: {status: ValidationFailure,
						messagePattern: fmt.Sprintf("Insufficient number of master host candidates: expected %d",
							common.MinMasterHostsNeededForInstallation)},
				}),
				errorExpected: false,
			},
			{
				name:               "pending-for-input to insufficient - not all hosts are ready to install",
				srcState:           models.ClusterStatusPendingForInput,
				dstState:           models.ClusterStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusInsufficient), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoInsufficient),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationFailure, messagePattern: "The cluster has hosts that are not ready to install."},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set."},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
				}),
				errorExpected: false,
			},
			{
				name:               "ready to pending-for-input - api vip not defined",
				srcState:           models.ClusterStatusReady,
				dstState:           models.ClusterStatusPendingForInput,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoPendingForInput),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationFailure, messagePattern: "API virtual IP is undefined"},
					isApiVipValid:                       {status: ValidationPending, messagePattern: "API virtual IP is undefined"},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "ingress vip 1.2.3.6 belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set."},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
				}),
				errorExpected: false,
			},
			{
				name:               "ready to pending-for-input - dns domain not defined",
				srcState:           models.ClusterStatusReady,
				dstState:           models.ClusterStatusPendingForInput,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoPendingForInput),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "ingress vip 1.2.3.6 belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationFailure, messagePattern: "The base domain is undefined and must be provided and must be provided"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set."},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
				}),
				errorExpected: false,
			},
			{
				name:               "ready to pending-for-input - pull secret not set",
				srcState:           models.ClusterStatusReady,
				dstState:           models.ClusterStatusPendingForInput,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      false,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoPendingForInput),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "ingress vip 1.2.3.6 belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationFailure, messagePattern: "The pull secret is not set."},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
				}),
				errorExpected: false,
			}, {
				name:               "pending-for-input to ready",
				srcState:           models.ClusterStatusPendingForInput,
				dstState:           models.ClusterStatusReady,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoReady),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set."},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
				}),
				errorExpected: false,
			},
			{
				name:               "insufficient to ready with disabled master",
				srcState:           models.ClusterStatusInsufficient,
				dstState:           models.ClusterStatusReady,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusDisabled), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
				},
				statusInfoChecker: makeValueChecker(statusInfoReady),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set."},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
				}),
				errorExpected: false,
			},
			{
				name:               "insufficient to ready",
				srcState:           models.ClusterStatusInsufficient,
				dstState:           models.ClusterStatusReady,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoReady),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set."},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
				}),
				errorExpected: false,
			},
			{
				name:               "ready to ready",
				srcState:           models.ClusterStatusReady,
				dstState:           models.ClusterStatusReady,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoReady),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
				}),
				errorExpected: false,
			},
			{
				name:               "finalizing to finalizing",
				srcState:           models.ClusterStatusFinalizing,
				srcStatusInfo:      statusInfoFinalizing,
				dstState:           models.ClusterStatusFinalizing,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker:  makeValueChecker(statusInfoFinalizing),
				validationsChecker: nil,
				errorExpected:      false,
			},
			{
				name:               "error to error",
				srcState:           models.ClusterStatusError,
				dstState:           models.ClusterStatusError,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker:  makeValueChecker(""),
				validationsChecker: nil,
				errorExpected:      false,
			},
			{
				name:               "installed to installed",
				srcState:           models.ClusterStatusInstalled,
				srcStatusInfo:      statusInfoInstalled,
				dstState:           models.ClusterStatusInstalled,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker:  makeValueChecker(statusInfoInstalled),
				validationsChecker: nil,
				errorExpected:      false,
			},
		}

		for i := range tests {
			t := tests[i]
			It(t.name, func() {
				cluster = common.Cluster{
					Cluster: models.Cluster{
						APIVip:                   t.apiVip,
						ID:                       &clusterId,
						IngressVip:               t.ingressVip,
						MachineNetworkCidr:       t.machineNetworkCidr,
						Status:                   &t.srcState,
						StatusInfo:               &t.srcStatusInfo,
						BaseDNSDomain:            t.dnsDomain,
						PullSecretSet:            t.pullSecretSet,
						ClusterNetworkCidr:       "1.3.0.0/16",
						ServiceNetworkCidr:       "1.4.0.0/16",
						ClusterNetworkHostPrefix: 24,
					},
				}
				Expect(db.Create(&cluster).Error).ShouldNot(HaveOccurred())
				for i := range t.hosts {
					t.hosts[i].ClusterID = clusterId
					Expect(db.Create(&t.hosts[i]).Error).ShouldNot(HaveOccurred())
				}
				cluster = getCluster(clusterId, db)
				if srcState != t.dstState {
					mockEvents.EXPECT().AddEvent(gomock.Any(), gomock.Any(), gomock.Any(),
						gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
				}
				if t.dstState == models.ClusterStatusInsufficient {
					mockHostAPIIsRequireUserActionResetFalse()
				}
				clusterAfterRefresh, err := clusterApi.RefreshStatus(ctx, &cluster, db)
				if t.errorExpected {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).ToNot(HaveOccurred())
				}
				Expect(clusterAfterRefresh.Status).To(Equal(&t.dstState))
				t.statusInfoChecker.check(clusterAfterRefresh.StatusInfo)
				if t.validationsChecker != nil {
					t.validationsChecker.check(clusterAfterRefresh.ValidationsInfo)
				}
			})
		}
	})
	AfterEach(func() {
		common.DeleteTestDB(db, dbName)
		ctrl.Finish()
	})
})

var _ = Describe("Refresh Cluster - Advanced networking validations", func() {
	var (
		ctx                                     = context.Background()
		db                                      *gorm.DB
		clusterId, hid1, hid2, hid3, hid4, hid5 strfmt.UUID
		cluster                                 common.Cluster
		clusterApi                              *Manager
		mockEvents                              *events.MockHandler
		mockHostAPI                             *host.MockAPI
		mockMetric                              *metrics.MockAPI
		ctrl                                    *gomock.Controller
		dbName                                  string = "cluster_transition_test_refresh_host_no_dhcp"
	)

	mockHostAPIIsRequireUserActionResetFalse := func() {
		mockHostAPI.EXPECT().IsRequireUserActionReset(gomock.Any()).Return(false).AnyTimes()
	}
	BeforeEach(func() {
		db = common.PrepareTestDB(dbName, &events.Event{})
		ctrl = gomock.NewController(GinkgoT())
		mockEvents = events.NewMockHandler(ctrl)
		mockHostAPI = host.NewMockAPI(ctrl)
		mockMetric = metrics.NewMockAPI(ctrl)
		clusterApi = NewManager(getDefaultConfig(), getTestLog().WithField("pkg", "cluster-monitor"), db,
			mockEvents, mockHostAPI, mockMetric, nil)

		hid1 = strfmt.UUID(uuid.New().String())
		hid2 = strfmt.UUID(uuid.New().String())
		hid3 = strfmt.UUID(uuid.New().String())
		hid4 = strfmt.UUID(uuid.New().String())
		hid5 = strfmt.UUID(uuid.New().String())
		clusterId = strfmt.UUID(uuid.New().String())
	})
	Context("All transitions", func() {
		var srcState string
		tests := []struct {
			name                     string
			srcState                 string
			srcStatusInfo            string
			machineNetworkCidr       string
			clusterNetworkCidr       string
			serviceNetworkCidr       string
			clusterNetworkHostPrefix int64
			apiVip                   string
			ingressVip               string
			dstState                 string
			hosts                    []models.Host
			statusInfoChecker        statusInfoChecker
			validationsChecker       *validationsChecker
			errorExpected            bool
		}{
			{
				name:               "pending-for-input to pending-for-input",
				srcState:           models.ClusterStatusPendingForInput,
				dstState:           models.ClusterStatusPendingForInput,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.4",
				ingressVip:         "1.2.3.5",
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoPendingForInput),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
					isClusterCidrDefined:                {status: ValidationFailure, messagePattern: "Cluster Network CIDR is undefined"},
					isServiceCidrDefined:                {status: ValidationFailure, messagePattern: "Service Network CIDR is undefined"},
					noCidrOverlapping:                   {status: ValidationPending, messagePattern: "At least one of the CIDRs .Machine Network, Cluster Network, Service Network. is undefined"},
					networkPrefixValid:                  {status: ValidationPending, messagePattern: "Cluster Network CIDR is undefined"},
				}),
				errorExpected: false,
			},
			{
				name:               "pending-for-input to insufficient - overlapping",
				srcState:           models.ClusterStatusPendingForInput,
				dstState:           models.ClusterStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.4",
				ingressVip:         "1.2.3.5",
				serviceNetworkCidr: "1.2.2.0/23",
				clusterNetworkCidr: "1.2.2.0/24",
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoInsufficient),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
					isClusterCidrDefined:                {status: ValidationSuccess, messagePattern: "Cluster Network CIDR is defined"},
					isServiceCidrDefined:                {status: ValidationSuccess, messagePattern: "Service Network CIDR is defined"},
					noCidrOverlapping:                   {status: ValidationFailure, messagePattern: "MachineNetworkCIDR and ServiceNetworkCIDR: CIDRS .* and .* overlap"},
					networkPrefixValid:                  {status: ValidationFailure, messagePattern: "Invalid Cluster Network prefix: Network prefix 0 is out of the allowed range"},
				}),
				errorExpected: false,
			},
			{
				name:                     "pending-for-input to ready",
				srcState:                 models.ClusterStatusPendingForInput,
				dstState:                 models.ClusterStatusReady,
				machineNetworkCidr:       "1.2.3.0/24",
				apiVip:                   "1.2.3.4",
				ingressVip:               "1.2.3.5",
				serviceNetworkCidr:       "1.2.8.0/23",
				clusterNetworkCidr:       "1.3.0.0/21",
				clusterNetworkHostPrefix: 23,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoReady),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
					isClusterCidrDefined:                {status: ValidationSuccess, messagePattern: "Cluster Network CIDR is defined"},
					isServiceCidrDefined:                {status: ValidationSuccess, messagePattern: "Service Network CIDR is defined"},
					noCidrOverlapping:                   {status: ValidationSuccess, messagePattern: "No CIDRS are overlapping"},
					networkPrefixValid:                  {status: ValidationSuccess, messagePattern: "Cluster Network prefix is valid."},
				}),
				errorExpected: false,
			},
			{
				name:                     "pending-for-input to insufficient (1)",
				srcState:                 models.ClusterStatusPendingForInput,
				dstState:                 models.ClusterStatusInsufficient,
				machineNetworkCidr:       "1.2.3.0/24",
				apiVip:                   "1.2.3.4",
				ingressVip:               "1.2.3.5",
				serviceNetworkCidr:       "1.2.8.0/23",
				clusterNetworkCidr:       "1.3.0.0/22",
				clusterNetworkHostPrefix: 23,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoInsufficient),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
					isClusterCidrDefined:                {status: ValidationSuccess, messagePattern: "Cluster Network CIDR is defined"},
					isServiceCidrDefined:                {status: ValidationSuccess, messagePattern: "Service Network CIDR is defined"},
					noCidrOverlapping:                   {status: ValidationSuccess, messagePattern: "No CIDRS are overlapping"},
					networkPrefixValid:                  {status: ValidationFailure, messagePattern: "does not contain enough addresses for"},
				}),
				errorExpected: false,
			},
			{
				name:                     "pending-for-input to insufficient (2)",
				srcState:                 models.ClusterStatusPendingForInput,
				dstState:                 models.ClusterStatusInsufficient,
				machineNetworkCidr:       "1.2.3.0/24",
				apiVip:                   "1.2.3.4",
				ingressVip:               "1.2.3.5",
				serviceNetworkCidr:       "1.2.8.0/23",
				clusterNetworkCidr:       "1.3.0.0/22",
				clusterNetworkHostPrefix: 24,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
					{ID: &hid5, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoInsufficient),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
					isClusterCidrDefined:                {status: ValidationSuccess, messagePattern: "Cluster Network CIDR is defined"},
					isServiceCidrDefined:                {status: ValidationSuccess, messagePattern: "Service Network CIDR is defined"},
					noCidrOverlapping:                   {status: ValidationSuccess, messagePattern: "No CIDRS are overlapping"},
					networkPrefixValid:                  {status: ValidationFailure, messagePattern: "does not contain enough addresses for"},
				}),
				errorExpected: false,
			},
			{
				name:                     "pending-for-input to insufficient (3)",
				srcState:                 models.ClusterStatusPendingForInput,
				dstState:                 models.ClusterStatusReady,
				machineNetworkCidr:       "1.2.3.0/24",
				apiVip:                   "1.2.3.4",
				ingressVip:               "1.2.3.5",
				serviceNetworkCidr:       "1.2.8.0/23",
				clusterNetworkCidr:       "1.3.0.0/21",
				clusterNetworkHostPrefix: 24,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
					{ID: &hid5, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoReady),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
					isClusterCidrDefined:                {status: ValidationSuccess, messagePattern: "Cluster Network CIDR is defined"},
					isServiceCidrDefined:                {status: ValidationSuccess, messagePattern: "Service Network CIDR is defined"},
					noCidrOverlapping:                   {status: ValidationSuccess, messagePattern: "No CIDRS are overlapping"},
					networkPrefixValid:                  {status: ValidationSuccess, messagePattern: "Cluster Network prefix is valid."},
				}),
				errorExpected: false,
			},
		}

		for i := range tests {
			t := tests[i]
			It(t.name, func() {
				cluster = common.Cluster{
					Cluster: models.Cluster{
						APIVip:                   t.apiVip,
						ID:                       &clusterId,
						IngressVip:               t.ingressVip,
						MachineNetworkCidr:       t.machineNetworkCidr,
						Status:                   &t.srcState,
						StatusInfo:               &t.srcStatusInfo,
						ClusterNetworkCidr:       t.clusterNetworkCidr,
						ServiceNetworkCidr:       t.serviceNetworkCidr,
						ClusterNetworkHostPrefix: t.clusterNetworkHostPrefix,
						PullSecretSet:            true,
						BaseDNSDomain:            "test.com",
					},
				}
				Expect(db.Create(&cluster).Error).ShouldNot(HaveOccurred())
				for i := range t.hosts {
					t.hosts[i].ClusterID = clusterId
					Expect(db.Create(&t.hosts[i]).Error).ShouldNot(HaveOccurred())
				}
				cluster = getCluster(clusterId, db)
				if srcState != t.dstState {
					mockEvents.EXPECT().AddEvent(gomock.Any(), gomock.Any(), gomock.Any(),
						gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
				}
				if t.dstState == models.ClusterStatusInsufficient {
					mockHostAPIIsRequireUserActionResetFalse()
				}
				clusterAfterRefresh, err := clusterApi.RefreshStatus(ctx, &cluster, db)
				if t.errorExpected {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).ToNot(HaveOccurred())
				}
				Expect(clusterAfterRefresh.Status).To(Equal(&t.dstState))
				t.statusInfoChecker.check(clusterAfterRefresh.StatusInfo)
				if t.validationsChecker != nil {
					t.validationsChecker.check(clusterAfterRefresh.ValidationsInfo)
				}
			})
		}
	})
	AfterEach(func() {
		common.DeleteTestDB(db, dbName)
		ctrl.Finish()
	})
})

var _ = Describe("Refresh Cluster - With DHCP", func() {
	var (
		ctx                               = context.Background()
		db                                *gorm.DB
		clusterId, hid1, hid2, hid3, hid4 strfmt.UUID
		cluster                           common.Cluster
		clusterApi                        *Manager
		mockEvents                        *events.MockHandler
		mockHostAPI                       *host.MockAPI
		mockMetric                        *metrics.MockAPI
		ctrl                              *gomock.Controller
		dbName                            string = "cluster_transition_test_refresh_host_with_dhcp"
	)

	mockHostAPIIsRequireUserActionResetFalse := func() {
		mockHostAPI.EXPECT().IsRequireUserActionReset(gomock.Any()).Return(false).AnyTimes()
	}
	BeforeEach(func() {
		db = common.PrepareTestDB(dbName, &events.Event{})
		ctrl = gomock.NewController(GinkgoT())
		mockEvents = events.NewMockHandler(ctrl)
		mockHostAPI = host.NewMockAPI(ctrl)
		mockMetric = metrics.NewMockAPI(ctrl)
		clusterApi = NewManager(getDefaultConfig(), getTestLog().WithField("pkg", "cluster-monitor"), db,
			mockEvents, mockHostAPI, mockMetric, nil)

		hid1 = strfmt.UUID(uuid.New().String())
		hid2 = strfmt.UUID(uuid.New().String())
		hid3 = strfmt.UUID(uuid.New().String())
		hid4 = strfmt.UUID(uuid.New().String())
		clusterId = strfmt.UUID(uuid.New().String())
	})
	Context("All transitions", func() {
		var srcState string
		tests := []struct {
			name                    string
			srcState                string
			srcStatusInfo           string
			machineNetworkCidr      string
			apiVip                  string
			ingressVip              string
			dnsDomain               string
			pullSecretSet           bool
			dstState                string
			hosts                   []models.Host
			statusInfoChecker       statusInfoChecker
			validationsChecker      *validationsChecker
			setMachineCidrUpdatedAt bool
			errorExpected           bool
		}{
			{
				name:               "pending-for-input to pending-for-input",
				srcState:           models.ClusterStatusPendingForInput,
				dstState:           models.ClusterStatusPendingForInput,
				machineNetworkCidr: "",
				apiVip:             "",
				ingressVip:         "",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Role: models.HostRoleMaster},
				},
				statusInfoChecker: makeValueChecker(statusInfoPendingForInput),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationFailure, messagePattern: "Machine Network CIDR is undefined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationPending, messagePattern: "Machine Network CIDR, API virtual IP, or Ingress virtual IP is undefined"},
					isApiVipDefined:                     {status: ValidationPending, messagePattern: "Machine Network CIDR is undefined"},
					isApiVipValid:                       {status: ValidationPending, messagePattern: "API virtual IP is undefined"},
					isIngressVipDefined:                 {status: ValidationPending, messagePattern: "Machine Network CIDR is undefined"},
					isIngressVipValid:                   {status: ValidationPending, messagePattern: "Ingress virtual IP is undefined"},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set."},
					SufficientMastersCount: {status: ValidationFailure,
						messagePattern: fmt.Sprintf("Insufficient number of master host candidates: expected %d",
							common.MinMasterHostsNeededForInstallation)},
				}),
				errorExpected: false,
			},
			{
				name:               "pending-for-input to insufficient - masters > 3",
				srcState:           models.ClusterStatusPendingForInput,
				dstState:           models.ClusterStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
				},
				statusInfoChecker: makeValueChecker(statusInfoInsufficient),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set."},
					SufficientMastersCount: {status: ValidationFailure,
						messagePattern: fmt.Sprintf("Insufficient number of master host candidates: expected %d",
							common.MinMasterHostsNeededForInstallation)},
				}),
				errorExpected: false,
			},
			{
				name:               "pending-for-input to insufficient - not all hosts are ready to install",
				srcState:           models.ClusterStatusPendingForInput,
				dstState:           models.ClusterStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusInsufficient), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoInsufficient),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationFailure, messagePattern: "The cluster has hosts that are not ready to install."},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set."},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
				}),
				errorExpected: false,
			},
			{
				name:               "ready to dhcp timeout - api vip not defined",
				srcState:           models.ClusterStatusReady,
				dstState:           models.ClusterStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoInsufficient),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationFailure, messagePattern: "API virtual IP is undefined; IP allocation from the DHCP server timed out"},
					isApiVipValid:                       {status: ValidationPending, messagePattern: "API virtual IP is undefined"},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "ingress vip 1.2.3.6 belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set."},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
				}),
				errorExpected: false,
			},
			{
				name:               "ready to insufficient - api vip not defined",
				srcState:           models.ClusterStatusReady,
				dstState:           models.ClusterStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoInsufficient),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationFailure, messagePattern: "API virtual IP is undefined; after the Machine Network CIDR has been defined, the API virtual IP is received from a DHCP lease allocation task which may take up to 2 minutes"},
					isApiVipValid:                       {status: ValidationPending, messagePattern: "API virtual IP is undefined"},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "ingress vip 1.2.3.6 belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set."},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
				}),
				setMachineCidrUpdatedAt: true,
				errorExpected:           false,
			},
			{
				name:               "dhcp timeout to ready",
				srcState:           models.ClusterStatusInsufficient,
				dstState:           models.ClusterStatusReady,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.7",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoReady),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "ingress vip 1.2.3.6 belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set."},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
				}),
				errorExpected: false,
			},
			{
				name:               "pending-for-input to ready",
				srcState:           models.ClusterStatusPendingForInput,
				dstState:           models.ClusterStatusReady,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoReady),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set."},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
				}),
				errorExpected: false,
			},
			{
				name:               "insufficient to ready",
				srcState:           models.ClusterStatusInsufficient,
				dstState:           models.ClusterStatusReady,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoReady),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set."},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
				}),
				errorExpected: false,
			},
			{
				name:               "ready to ready",
				srcState:           models.ClusterStatusReady,
				dstState:           models.ClusterStatusReady,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoReady),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set."},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
				}),
				errorExpected: false,
			},
			{
				name:               "finalizing to finalizing",
				srcState:           models.ClusterStatusFinalizing,
				srcStatusInfo:      statusInfoFinalizing,
				dstState:           models.ClusterStatusFinalizing,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker:  makeValueChecker(statusInfoFinalizing),
				validationsChecker: nil,
				errorExpected:      false,
			},
			{
				name:               "error to error",
				srcState:           models.ClusterStatusError,
				dstState:           models.ClusterStatusError,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker:  makeValueChecker(""),
				validationsChecker: nil,
				errorExpected:      false,
			},
			{
				name:               "installed to installed",
				srcState:           models.ClusterStatusInstalled,
				srcStatusInfo:      statusInfoInstalled,
				dstState:           models.ClusterStatusInstalled,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventory(), Role: models.HostRoleWorker},
				},
				statusInfoChecker:  makeValueChecker(statusInfoInstalled),
				validationsChecker: nil,
				errorExpected:      false,
			},
		}

		for i := range tests {
			t := tests[i]
			It(t.name, func() {
				cluster = common.Cluster{
					Cluster: models.Cluster{
						APIVip:                   t.apiVip,
						ID:                       &clusterId,
						IngressVip:               t.ingressVip,
						MachineNetworkCidr:       t.machineNetworkCidr,
						Status:                   &t.srcState,
						StatusInfo:               &t.srcStatusInfo,
						VipDhcpAllocation:        swag.Bool(true),
						BaseDNSDomain:            t.dnsDomain,
						PullSecretSet:            t.pullSecretSet,
						ServiceNetworkCidr:       "1.2.4.0/24",
						ClusterNetworkCidr:       "1.3.0.0/16",
						ClusterNetworkHostPrefix: 24,
					},
				}
				if t.setMachineCidrUpdatedAt {
					cluster.MachineNetworkCidrUpdatedAt = time.Now()
				} else {
					cluster.MachineNetworkCidrUpdatedAt = time.Now().Add(-3 * time.Minute)
				}
				Expect(db.Create(&cluster).Error).ShouldNot(HaveOccurred())
				for i := range t.hosts {
					t.hosts[i].ClusterID = clusterId
					Expect(db.Create(&t.hosts[i]).Error).ShouldNot(HaveOccurred())
				}
				cluster = getCluster(clusterId, db)
				if srcState != t.dstState {
					mockEvents.EXPECT().AddEvent(gomock.Any(), gomock.Any(), gomock.Any(),
						gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
				}
				if t.dstState == models.ClusterStatusInsufficient {
					mockHostAPIIsRequireUserActionResetFalse()
				}
				clusterAfterRefresh, err := clusterApi.RefreshStatus(ctx, &cluster, db)
				if t.errorExpected {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).ToNot(HaveOccurred())
				}
				Expect(clusterAfterRefresh.Status).To(Equal(&t.dstState))
				t.statusInfoChecker.check(clusterAfterRefresh.StatusInfo)
				if t.validationsChecker != nil {
					t.validationsChecker.check(clusterAfterRefresh.ValidationsInfo)
				}
			})
		}
	})
	AfterEach(func() {
		common.DeleteTestDB(db, dbName)
		ctrl.Finish()
	})
})

var _ = Describe("NTP refresh cluster", func() {
	var (
		ctx                                     = context.Background()
		db                                      *gorm.DB
		clusterId, hid1, hid2, hid3, hid4, hid5 strfmt.UUID
		cluster                                 common.Cluster
		clusterApi                              *Manager
		mockEvents                              *events.MockHandler
		mockHostAPI                             *host.MockAPI
		mockMetric                              *metrics.MockAPI
		ctrl                                    *gomock.Controller
		dbName                                  string = "cluster_transition_test_refresh_cluster_with_ntp"
	)

	mockHostAPIIsRequireUserActionResetFalse := func() {
		mockHostAPI.EXPECT().IsRequireUserActionReset(gomock.Any()).Return(false).AnyTimes()
	}
	BeforeEach(func() {
		db = common.PrepareTestDB(dbName, &events.Event{})
		ctrl = gomock.NewController(GinkgoT())
		mockEvents = events.NewMockHandler(ctrl)
		mockHostAPI = host.NewMockAPI(ctrl)
		mockMetric = metrics.NewMockAPI(ctrl)
		clusterApi = NewManager(getDefaultConfig(), getTestLog().WithField("pkg", "cluster-monitor"), db,
			mockEvents, mockHostAPI, mockMetric, nil)
		hid1 = strfmt.UUID(uuid.New().String())
		hid2 = strfmt.UUID(uuid.New().String())
		hid3 = strfmt.UUID(uuid.New().String())
		hid4 = strfmt.UUID(uuid.New().String())
		hid5 = strfmt.UUID(uuid.New().String())
		clusterId = strfmt.UUID(uuid.New().String())
	})
	Context("All transitions", func() {
		var srcState string
		tests := []struct {
			name                    string
			srcState                string
			srcStatusInfo           string
			machineNetworkCidr      string
			apiVip                  string
			ingressVip              string
			dnsDomain               string
			pullSecretSet           bool
			dstState                string
			hosts                   []models.Host
			statusInfoChecker       statusInfoChecker
			validationsChecker      *validationsChecker
			setMachineCidrUpdatedAt bool
			errorExpected           bool
		}{
			{
				name:               "pending-for-input to insufficient - ntp problem",
				srcState:           models.ClusterStatusPendingForInput,
				dstState:           models.ClusterStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239 - 400), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
				},
				statusInfoChecker: makeValueChecker(statusInfoInsufficient),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "The Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "The Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "The API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "The Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set"},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates"},
					IsNtpServerConfigured:               {status: ValidationFailure, messagePattern: "please configure an NTP server via DHCP"},
				}),
				errorExpected: false,
			},
			{
				name:               "pending-for-input to ready",
				srcState:           models.ClusterStatusPendingForInput,
				dstState:           models.ClusterStatusReady,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
				},
				statusInfoChecker: makeValueChecker(statusInfoReady),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "The Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "The Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "The API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "The Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set"},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates"},
					IsNtpServerConfigured:               {status: ValidationSuccess, messagePattern: "No ntp problems found"},
				}),
				errorExpected: false,
			},
			{
				name:               "insufficient to ready",
				srcState:           models.ClusterStatusInsufficient,
				dstState:           models.ClusterStatusReady,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
				},
				statusInfoChecker: makeValueChecker(statusInfoReady),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "The Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "The Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "The API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "The Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set"},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates"},
					IsNtpServerConfigured:               {status: ValidationSuccess, messagePattern: "No ntp problems found"},
				}),
				errorExpected: false,
			},
			{
				name:               "ready to ready",
				srcState:           models.ClusterStatusReady,
				dstState:           models.ClusterStatusReady,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
				},
				statusInfoChecker: makeValueChecker(statusInfoReady),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "The Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "The Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "The API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "The Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set"},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates"},
					IsNtpServerConfigured:               {status: ValidationSuccess, messagePattern: "No ntp problems found"},
				}),
				errorExpected: false,
			},

			{
				name:               "ready to ready with disabled",
				srcState:           models.ClusterStatusReady,
				dstState:           models.ClusterStatusReady,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusDisabled), Inventory: defaultInventoryWithTimestamp(1601909239 + 1000), Role: models.HostRoleWorker},
					{ID: &hid5, Status: swag.String(models.HostStatusDisabled), Inventory: defaultInventoryWithTimestamp(1601909239 - 1000), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoReady),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "The Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "The Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "The API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "The Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set"},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates"},
					IsNtpServerConfigured:               {status: ValidationSuccess, messagePattern: "No ntp problems found"},
				}),
				errorExpected: false,
			},

			{
				name:               "ready to insufficient with disconnected",
				srcState:           models.ClusterStatusReady,
				dstState:           models.ClusterStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusDisconnected), Inventory: defaultInventoryWithTimestamp(1601909239 + 1000), Role: models.HostRoleWorker},
					{ID: &hid5, Status: swag.String(models.HostStatusDisconnected), Inventory: defaultInventoryWithTimestamp(1601909239 - 1000), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoInsufficient),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "The Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "The Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "The API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "The Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationFailure, messagePattern: "The cluster has hosts that are not ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set"},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates"},
					IsNtpServerConfigured:               {status: ValidationSuccess, messagePattern: "No ntp problems found"},
				}),
				errorExpected: false,
			},

			{
				name:               "ready to insufficient with needs o be rebooted status",
				srcState:           models.ClusterStatusReady,
				dstState:           models.ClusterStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
					{ID: &hid4, Status: swag.String(models.HostStatusResettingPendingUserAction), Inventory: defaultInventoryWithTimestamp(1601909239 + 1000), Role: models.HostRoleWorker},
					{ID: &hid5, Status: swag.String(models.HostStatusResettingPendingUserAction), Inventory: defaultInventoryWithTimestamp(1601909239 - 1000), Role: models.HostRoleWorker},
				},
				statusInfoChecker: makeValueChecker(statusInfoInsufficient),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "The Machine Network CIDR is defined."},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "The Cluster Machine CIDR is equivalent to the calculated CIDR."},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "The API virtual IP is defined."},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "api vip 1.2.3.5 belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "The Ingress virtual IP is defined."},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "ingress vip 1.2.3.6 belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationFailure, messagePattern: "The cluster has hosts that are not ready to install."},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined."},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set."},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates."},
					IsNtpServerConfigured:               {status: ValidationSuccess, messagePattern: "No ntp problems found"},
				}),
				errorExpected: false,
			},

			{
				name:               "ready to insufficient",
				srcState:           models.ClusterStatusReady,
				dstState:           models.ClusterStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				apiVip:             "1.2.3.5",
				ingressVip:         "1.2.3.6",
				dnsDomain:          "test.com",
				pullSecretSet:      true,
				hosts: []models.Host{
					{ID: &hid1, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
					{ID: &hid2, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239 - 400), Role: models.HostRoleMaster},
					{ID: &hid3, Status: swag.String(models.HostStatusKnown), Inventory: defaultInventoryWithTimestamp(1601909239), Role: models.HostRoleMaster},
				},
				statusInfoChecker: makeValueChecker(statusInfoInsufficient),
				validationsChecker: makeJsonChecker(map[validationID]validationCheckResult{
					IsMachineCidrDefined:                {status: ValidationSuccess, messagePattern: "The Machine Network CIDR is defined"},
					isMachineCidrEqualsToCalculatedCidr: {status: ValidationSuccess, messagePattern: "The Cluster Machine CIDR is equivalent to the calculated CIDR"},
					isApiVipDefined:                     {status: ValidationSuccess, messagePattern: "The API virtual IP is defined"},
					isApiVipValid:                       {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					isIngressVipDefined:                 {status: ValidationSuccess, messagePattern: "The Ingress virtual IP is defined"},
					isIngressVipValid:                   {status: ValidationSuccess, messagePattern: "belongs to the Machine CIDR and is not in use."},
					AllHostsAreReadyToInstall:           {status: ValidationSuccess, messagePattern: "All hosts in the cluster are ready to install"},
					IsDNSDomainDefined:                  {status: ValidationSuccess, messagePattern: "The base domain is defined"},
					IsPullSecretSet:                     {status: ValidationSuccess, messagePattern: "The pull secret is set"},
					SufficientMastersCount:              {status: ValidationSuccess, messagePattern: "The cluster has a sufficient number of master candidates"},
					IsNtpServerConfigured:               {status: ValidationFailure, messagePattern: "please configure an NTP server via DHCP"},
				}),
				errorExpected: false,
			},
		}
		for i := range tests {
			t := tests[i]
			It(t.name, func() {
				cluster = common.Cluster{
					Cluster: models.Cluster{
						APIVip:                   t.apiVip,
						ID:                       &clusterId,
						IngressVip:               t.ingressVip,
						MachineNetworkCidr:       t.machineNetworkCidr,
						Status:                   &t.srcState,
						StatusInfo:               &t.srcStatusInfo,
						BaseDNSDomain:            t.dnsDomain,
						PullSecretSet:            t.pullSecretSet,
						ClusterNetworkCidr:       "1.3.0.0/16",
						ServiceNetworkCidr:       "1.4.0.0/16",
						ClusterNetworkHostPrefix: 24,
					},
				}
				Expect(db.Create(&cluster).Error).ShouldNot(HaveOccurred())
				for i := range t.hosts {
					t.hosts[i].ClusterID = clusterId
					Expect(db.Create(&t.hosts[i]).Error).ShouldNot(HaveOccurred())
				}
				cluster = getCluster(clusterId, db)
				if srcState != t.dstState {
					mockEvents.EXPECT().AddEvent(gomock.Any(), gomock.Any(), gomock.Any(),
						gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
				}
				mockHostAPIIsRequireUserActionResetFalse()
				if t.dstState == models.ClusterStatusInsufficient {
					mockHostAPIIsRequireUserActionResetFalse()
				}
				clusterAfterRefresh, err := clusterApi.RefreshStatus(ctx, &cluster, db)
				if t.errorExpected {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).ToNot(HaveOccurred())
				}
				Expect(clusterAfterRefresh.Status).To(Equal(&t.dstState))
				t.statusInfoChecker.check(clusterAfterRefresh.StatusInfo)
				if t.validationsChecker != nil {
					t.validationsChecker.check(clusterAfterRefresh.ValidationsInfo)
				}
			})
		}
	})
	AfterEach(func() {
		common.DeleteTestDB(db, dbName)
		ctrl.Finish()
	})
})

func getCluster(clusterId strfmt.UUID, db *gorm.DB) common.Cluster {
	var cluster common.Cluster
	Expect(db.Preload("Hosts").First(&cluster, "id = ?", clusterId).Error).ShouldNot(HaveOccurred())
	return cluster
}
