package models

import "time"

type ReleaseStatus int

const (
	ReleasePending  ReleaseStatus = iota
	ReleaseUpToDate
	ReleaseInvalid
)

type ProjectReleaseInfo struct {
	Name              string
	Path              string
	Status            ReleaseStatus
	LatestTag         string
	CommitsAhead      int
	LastDevCommitDate time.Time
	InvalidReason     string
	DevBranch         string
	MasterBranch      string
}

type ReleaseReport struct {
	Pending     []ProjectReleaseInfo
	Released    []ProjectReleaseInfo
	Invalid     []ProjectReleaseInfo
	GeneratedAt time.Time
}
