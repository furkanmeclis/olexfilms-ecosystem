package queue

import (
	"errors"
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

type fakeRegistrar struct {
	entries []registered
	failOn  string
}

type registered struct {
	cron, taskType, queue string
}

func (f *fakeRegistrar) Register(cron string, task *asynq.Task, opts ...asynq.Option) (string, error) {
	if task.Type() == f.failOn {
		return "", errors.New("boom")
	}
	q := "default"
	for _, o := range opts {
		if o.Type() == asynq.QueueOpt {
			q, _ = o.Value().(string)
		}
	}
	f.entries = append(f.entries, registered{cron: cron, taskType: task.Type(), queue: q})
	return "id", nil
}

// The worker-core scheduler registers the log purge sweep and the daily
// exchange rate fetch (evening run + morning catch-up) on maintenance.
func TestRegisterSchedulesIncludesRatesFetch(t *testing.T) {
	r := &fakeRegistrar{}
	if err := RegisterSchedules(r, nil); err != nil {
		t.Fatal(err)
	}
	want := map[registered]bool{
		{cron: logPurgeCron, taskType: TaskLogPurgeSweep, queue: QueueMaintenance}:                      false,
		{cron: ratesFetchCron, taskType: TaskRatesFetch, queue: QueueMaintenance}:                       false,
		{cron: ratesFetchCatchUpCron, taskType: TaskRatesFetch, queue: QueueMaintenance}:                false,
		{cron: warrantyExpireCron, taskType: TaskWarrantyExpire, queue: QueueMaintenance}:               false,
		{cron: warrantyExpiringScanCron, taskType: TaskWarrantyExpiringScan, queue: QueueMaintenance}:   false,
		{cron: warrantyRepairScanCron, taskType: TaskWarrantyRepairScan, queue: QueueMaintenance}:       false,
		{cron: inventoryRebuildCron, taskType: TaskInventoryRebuild, queue: QueueMaintenance}:           false,
		{cron: vehicleTransferExpireCron, taskType: TaskVehicleTransferExpire, queue: QueueMaintenance}: false,
		{cron: tasksDueScanCron, taskType: TaskTasksDueScan, queue: QueueMaintenance}:                   false,
		{cron: appointmentNoShowScanCron, taskType: TaskAppointmentNoShowScan, queue: QueueMaintenance}: false,
		{cron: quoteExpireCron, taskType: TaskQuoteExpire, queue: QueueMaintenance}:                     false,
		{cron: warehouseEODCron, taskType: TaskWarehouseEODReports, queue: QueueMaintenance}:            false,
		{cron: glorianPullCron, taskType: TaskGlorianPullCatalog, queue: QueueMaintenance}:              false,
		{cron: glorianPushCron, taskType: TaskGlorianPushBarcodes, queue: QueueMaintenance}:             false,
		{cron: glorianOrderReplayCron, taskType: TaskGlorianOrderReplay, queue: QueueMaintenance}:       false,
		{cron: staffPaymentsPostDueCron, taskType: TaskStaffPaymentsPostDue, queue: QueueMaintenance}:   false,
		// TEC-393.
		{cron: conversationAIRunPurgeCron, taskType: TaskConversationAIRunPurge, queue: QueueMaintenance}: false,
		{cron: oauthCleanupCron, taskType: TaskOAuthCleanup, queue: QueueMaintenance}:                     false,
		// TEC-395.
		{cron: whatsAppQueueSweepCron, taskType: TaskWhatsAppQueueSweep, queue: QueueMaintenance}: false,
		// TEC-387.
		{cron: aiActionSweepCron, taskType: TaskAIActionSweep, queue: QueueMaintenance}: false,
		// TEC-407.
		{cron: campaignTickCron, taskType: TaskCampaignTick, queue: QueueMaintenance}: false,
		// TEC-481.
		{cron: certificateExpiryScanCron, taskType: TaskCertificateExpiryScan, queue: QueueMaintenance}: false,
		// TEC-476.
		{cron: fleetReportsScheduleCron, taskType: TaskFleetReportsSchedule, queue: QueueMaintenance}: false,
	}
	for _, e := range r.entries {
		if _, ok := want[e]; !ok {
			t.Errorf("unexpected schedule %+v", e)
		}
		want[e] = true
	}
	for e, seen := range want {
		if !seen {
			t.Errorf("missing schedule %+v", e)
		}
	}
}

func TestRegisterSchedulesSurfacesErrors(t *testing.T) {
	if err := RegisterSchedules(&fakeRegistrar{failOn: TaskRatesFetch}, nil); err == nil {
		t.Fatal("a failed registration must surface")
	}
}

func TestSchedulerTimezoneLoads(t *testing.T) {
	if _, err := time.LoadLocation(SchedulerTimezone); err != nil {
		t.Fatal(err)
	}
}

func TestRatesFetchTaskRetries(t *testing.T) {
	for _, p := range Schedules() {
		if p.Type != TaskRatesFetch {
			continue
		}
		var retry bool
		for _, o := range p.Opts {
			if o.Type() == asynq.MaxRetryOpt {
				retry = o.Value().(int) > 0
			}
		}
		if !retry {
			t.Fatalf("rates fetch must retry: %+v", p)
		}
	}
}

func TestInventoryRebuildPayload(t *testing.T) {
	task, err := NewInventoryRebuildTask(42)
	if err != nil {
		t.Fatal(err)
	}
	if task.Type() != TaskInventoryRebuild {
		t.Fatalf("type = %s", task.Type())
	}
	p, err := ParseInventoryRebuildPayload(task.Payload())
	if err != nil || p.OrganizationID != 42 {
		t.Fatalf("payload = %+v, %v", p, err)
	}
	if p, err := ParseInventoryRebuildPayload(nil); err != nil || p.OrganizationID != 0 {
		t.Fatalf("empty payload = %+v, %v", p, err)
	}
	if _, err := ParseInventoryRebuildPayload([]byte(`{"organization_id":-1}`)); err == nil {
		t.Fatal("negative organization must be rejected")
	}
}
