package funding

import (
	"context"
	"time"
)

type SnapshotRateProvider struct {
	repo Repository
}

func NewSnapshotRateProvider(repo Repository) *SnapshotRateProvider {
	return &SnapshotRateProvider{repo: repo}
}

func (p *SnapshotRateProvider) GetRates(ctx context.Context, symbol Symbol, _ time.Time, windowEnd time.Time) ([]SourceRate, error) {
	snapshots, err := p.repo.ListLatestRateSnapshots(ctx, symbol.ID, windowEnd.UTC())
	if err != nil {
		return nil, err
	}
	out := make([]SourceRate, 0, len(snapshots))
	for _, snapshot := range snapshots {
		out = append(out, SourceRate{
			SourceName:      snapshot.SourceName,
			Rate:            snapshot.Rate,
			IntervalSeconds: snapshot.IntervalSeconds,
		})
	}
	return out, nil
}
