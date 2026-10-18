package booking

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"register_manager/internal/apperr"
	"register_manager/internal/auth"
	"register_manager/internal/schedule"
	"register_manager/internal/storage"
)

// StatisticsInput selects an inclusive range of instance-local dates.
type StatisticsInput struct {
	From, To string
	Basis    string
}

type StatusCounts struct {
	Booked    int64 `json:"booked"`
	Completed int64 `json:"completed"`
	Cancelled int64 `json:"cancelled"`
	NoShow    int64 `json:"no_show"`
}

type StatisticsSummary struct {
	StatusCounts
	Total           int64 `json:"total"`
	UniqueCustomers int64 `json:"unique_customers"`
}

type StatisticsBucket struct {
	From   string       `json:"from"`
	To     string       `json:"to"`
	Counts StatusCounts `json:"counts"`
}

// Statistics reflects current appointment records, not historical state snapshots.
type Statistics struct {
	From        string             `json:"from"`
	To          string             `json:"to"`
	Basis       string             `json:"basis" enum:"starts_at,created_at"`
	Timezone    string             `json:"timezone"`
	Granularity string             `json:"granularity" enum:"day,week,month"`
	Summary     StatisticsSummary  `json:"summary"`
	Series      []StatisticsBucket `json:"series"`
}

func statisticsBuckets(from, to schedule.Date) (string, []StatisticsBucket, map[schedule.Date]int) {
	grain := "day"
	if days := to.Sub(from) + 1; days > 180 {
		grain = "month"
	} else if days > 31 {
		grain = "week"
	}
	buckets := []StatisticsBucket{}
	indices := make(map[schedule.Date]int, to.Sub(from)+1)
	for start := from; start <= to; {
		next := start.AddDays(1)
		switch grain {
		case "week":
			next = start.WeekStart().AddDays(7)
		case "month":
			year, month, _ := start.YMD()
			next = schedule.NewDate(year, month+1, 1)
		}
		end := min(next.AddDays(-1), to)
		for day := start; day <= end; day++ {
			indices[day] = len(buckets)
		}
		buckets = append(buckets, StatisticsBucket{From: start.String(), To: end.String()})
		start = next
	}
	return grain, buckets, indices
}

// Statistics queries both summaries in one authorized read snapshot. Dates are
// resolved with the instance's IANA zone, including short and long DST days.
func (s *Service) Statistics(ctx context.Context, actor *auth.Account, instanceID string, in StatisticsInput) (Statistics, error) {
	var out Statistics
	err := auth.InstanceRead(ctx, s.DB, actor, instanceID, func(tx bun.Tx, acc *auth.Access) error {
		from, err := schedule.ParseDate(in.From)
		if err != nil {
			return apperr.Invalid("invalid_date")
		}
		to, err := schedule.ParseDate(in.To)
		if err != nil {
			return apperr.Invalid("invalid_date")
		}
		if to < from || to.Sub(from) >= 366 {
			return apperr.Invalid("invalid_statistics_range")
		}
		// Only these identifiers can enter SQL; never interpolate a query parameter.
		var column bun.Ident
		switch in.Basis {
		case "starts_at":
			column = bun.Ident("starts_at")
		case "created_at":
			column = bun.Ident("created_at")
		default:
			return apperr.Invalid("invalid_statistics_basis")
		}
		loc, err := loadLocation(acc.Instance)
		if err != nil {
			return err
		}
		lower := schedule.DayStart(loc, from).UnixMilli()
		upper := schedule.DayStart(loc, to.AddDays(1)).UnixMilli()
		query := func() *bun.SelectQuery {
			return tx.NewSelect().Model((*storage.Appointment)(nil)).
				Where("instance_id = ?", instanceID).
				Where("? >= ? AND ? < ?", column, lower, column, upper)
		}
		out = Statistics{From: from.String(), To: to.String(), Basis: in.Basis, Timezone: acc.Instance.Timezone}
		if err := query().
			ColumnExpr("COUNT(*) AS total").
			ColumnExpr("COUNT(DISTINCT customer_id) AS unique_customers").
			ColumnExpr("COUNT(CASE WHEN status = 'booked' THEN 1 END) AS booked").
			ColumnExpr("COUNT(CASE WHEN status = 'completed' THEN 1 END) AS completed").
			ColumnExpr("COUNT(CASE WHEN status = 'cancelled' THEN 1 END) AS cancelled").
			ColumnExpr("COUNT(CASE WHEN status = 'no_show' THEN 1 END) AS no_show").
			Scan(ctx, &out.Summary); err != nil {
			return err
		}
		grain, buckets, indices := statisticsBuckets(from, to)
		out.Granularity, out.Series = grain, buckets
		// Stream two scalar columns. Memory depends on the number of buckets,
		// not appointments; no customer identities leave this service.
		rows, err := query().ColumnExpr("?", column).Column("status").Rows(ctx)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var at int64
			var status string
			if err := rows.Scan(&at, &status); err != nil {
				return err
			}
			day := schedule.DateIn(time.UnixMilli(at), loc)
			index, ok := indices[day]
			if !ok {
				return fmt.Errorf("statistics date outside range: %s", day)
			}
			counts := &out.Series[index].Counts
			switch status {
			case "booked":
				counts.Booked++
			case "completed":
				counts.Completed++
			case "cancelled":
				counts.Cancelled++
			case "no_show":
				counts.NoShow++
			default:
				return fmt.Errorf("unknown appointment status: %s", status)
			}
		}
		return rows.Err()
	})
	return out, err
}
