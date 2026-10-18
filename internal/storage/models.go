package storage

import "github.com/uptrace/bun"

// Database models map the existing Goose schema. Times are UTC Unix milliseconds.
type Account struct {
	bun.BaseModel      `bun:"table:accounts,alias:accounts"`
	ID                 string `bun:",pk"`
	Login              string
	Name               string
	PasswordHash       string
	AuthVersion        int64
	DisabledAt         *int64
	Locale             *string
	SuperAdmin         bool
	MustChangePassword bool
	CreatedAt          int64
	UpdatedAt          int64
}

type Appointment struct {
	bun.BaseModel `bun:"table:appointments,alias:appointments"`
	ID            string `bun:",pk"`
	InstanceID    string
	CustomerID    string
	StartsAt      int64
	EndsAt        int64
	Status        string
	CreatedAt     int64
	UpdatedAt     int64
}

type AuditLog struct {
	bun.BaseModel   `bun:"table:audit_log,alias:audit_log"`
	ID              int64 `bun:",pk"`
	At              int64
	Scope           string
	InstanceID      *string
	ActorKind       string
	ActorAccountID  *string
	ActorCustomerID *string
	Action          string
	TargetType      *string
	TargetID        *string
	BeforeJson      *string
	AfterJson       *string
}

type BlockedPeriod struct {
	bun.BaseModel `bun:"table:blocked_periods,alias:blocked_periods"`
	ID            string `bun:",pk"`
	InstanceID    string
	StartsAt      int64
	EndsAt        int64
	Reason        *string
	CreatedAt     int64
}

type BookingPolicy struct {
	bun.BaseModel                `bun:"table:booking_policies,alias:booking_policies"`
	InstanceID                   string `bun:",pk"`
	SlotStepMinutes              int64
	ServiceDurationMinutes       int64
	BufferMinutes                int64
	DailyCustomerLimit           *int
	PerCustomerDailyLimit        *int
	PerCustomerWeeklyLimit       *int
	MaxFutureBookings            *int
	BookingHorizonDays           int64
	MinBookingNoticeMinutes      int64
	MinCancellationNoticeMinutes int64
	UpdatedAt                    int64
}

type Customer struct {
	bun.BaseModel     `bun:"table:customers,alias:customers"`
	ID                string `bun:",pk"`
	InstanceID        string
	Name              string
	Contact           *string
	Status            string
	InternalNote      *string
	Locale            *string
	AccessTokenDigest *string
	AuthVersion       int64
	CreatedAt         int64
	UpdatedAt         int64
}

type Instance struct {
	bun.BaseModel          `bun:"table:instances,alias:instances"`
	ID                     string `bun:",pk"`
	Slug                   string
	Name                   string
	Status                 string
	Timezone               string
	DefaultLocale          string
	RegistrationInviteCode string
	ContactKind            string
	ContactRequired        bool
	BrandColor             *string
	CreatedAt              int64
	UpdatedAt              int64
}

type InstanceOperator struct {
	bun.BaseModel `bun:"table:instance_operators,alias:instance_operators"`
	InstanceID    string `bun:",pk"`
	AccountID     string `bun:",pk"`
	CreatedAt     int64
}

type PlatformSetting struct {
	bun.BaseModel `bun:"table:platform_settings,alias:platform_settings"`
	ID            int64 `bun:",pk"`
	MaxCycleDays  int64
	UpdatedAt     int64
}

type Schedule struct {
	bun.BaseModel   `bun:"table:schedules,alias:schedules"`
	InstanceID      string `bun:",pk"`
	CycleLengthDays int64
	AnchorDate      string
	CycleDays       string
	UpdatedAt       int64
}

type ScheduleOverride struct {
	bun.BaseModel `bun:"table:schedule_overrides,alias:schedule_overrides"`
	InstanceID    string `bun:",pk"`
	Date          string `bun:",pk"`
	Periods       string
	UpdatedAt     int64
}
