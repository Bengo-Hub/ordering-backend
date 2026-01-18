package sla

import "errors"

var (
	// ErrMetricNotFound is returned when an SLA metric is not found.
	ErrMetricNotFound = errors.New("sla: metric not found")

	// ErrMetricAlreadyExists is returned when trying to create a duplicate metric.
	ErrMetricAlreadyExists = errors.New("sla: metric already exists for this order and type")

	// ErrMetricAlreadyCompleted is returned when trying to complete an already completed metric.
	ErrMetricAlreadyCompleted = errors.New("sla: metric already completed")

	// ErrInvalidMetricType is returned for unknown metric types.
	ErrInvalidMetricType = errors.New("sla: invalid metric type")

	// ErrInvalidStatus is returned for invalid status transitions.
	ErrInvalidStatus = errors.New("sla: invalid status")
)
