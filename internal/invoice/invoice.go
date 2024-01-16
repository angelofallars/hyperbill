package invoice

import "time"

type Invoice struct {
	StartDate  time.Time
	EndDate    time.Time
	T5Report   CategoryReport
	T4Report   CategoryReport
	T3Report   CategoryReport
	T2Report   CategoryReport
	T1Report   CategoryReport
	TotalPrice float64
}

type CategoryReport struct {
	PricePerHour float64
	TimeSpent    time.Duration
	Price        float64
}

type Category string

const (
	CategoryT1 Category = "T1"
	CategoryT2 Category = "T2"
	CategoryT3 Category = "T3"
	CategoryT4 Category = "T4"
	CategoryT5 Category = "T5"
)
