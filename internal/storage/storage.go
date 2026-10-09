// Package storage declares the persistence interfaces the bot core uses, one
// small interface per aggregate (one file each). Methods are named by intent,
// not by SQL. The only implementation is package storage/sqlite.
//
// Times cross this boundary as time.Time and are stored as UTC unix seconds.
// Callers compute Jalali month ranges and pass plain time ranges.
package storage
