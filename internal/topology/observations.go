package topology

// This file defines observations topology observation contracts.

import (
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
)

// Observation represents observation data used by the package.
type Observation struct {
	ID              uuid.UUID
	LocalDeviceID   uuid.UUID
	RemoteIdentity  string
	RemoteDeviceID  uuid.UUID
	LocalPort       string
	RemotePort      string
	Protocol        domain.DiscoveryProtocol
	SelfNeighbor    bool
	FirstObservedAt time.Time
	LastObservedAt  time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// UnresolvedNeighbor represents unresolved neighbor data used by the package.
type UnresolvedNeighbor struct {
	ID              uuid.UUID
	LocalDeviceID   uuid.UUID
	RemoteIdentity  string
	Protocol        domain.DiscoveryProtocol
	Occurrences     int
	FirstObservedAt time.Time
	LastObservedAt  time.Time
	ResolvedAt      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ObservationStore defines the observation store contract for the package.
type ObservationStore interface {
	UpsertObservation(*Observation) error
	PruneLocalObservations(localDeviceID uuid.UUID, protocols []domain.DiscoveryProtocol, keep []Observation) (int, error)
	ListObservationsForDevices([]uuid.UUID) ([]Observation, error)
	UpsertUnresolvedNeighbor(*UnresolvedNeighbor) error
	ResolveUnresolvedNeighbor(localDeviceID uuid.UUID, remoteIdentity string, protocol domain.DiscoveryProtocol, resolvedAt time.Time) error
	GetUnresolvedNeighborsByDeviceID(localDeviceID uuid.UUID) ([]UnresolvedNeighbor, error)
}

// LinkWriter defines the link writer contract for the package.
type LinkWriter interface {
	UpsertDetailed(*domain.Link) (domain.LinkUpsertResult, error)
}

// ApplyEvent represents apply event data used by the package.
type ApplyEvent struct {
	Link   domain.Link
	Result domain.LinkUpsertResult
}

// ApplyResult represents apply result data used by the package.
type ApplyResult struct {
	TopologyChanged bool
	LinksCreated    int
	Events          []ApplyEvent
}

// NormalizeRemoteIdentity returns a normalized remote identity value for the package.
func NormalizeRemoteIdentity(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.TrimSuffix(normalized, ".")
	if index := strings.Index(normalized, "."); index >= 0 {
		normalized = normalized[:index]
	}
	return normalized
}

func CandidateLinks(observations []Observation) []domain.Link {
	candidates := make([]domain.Link, 0, len(observations))
	seen := make(map[string]struct{}, len(observations))

	sort.Slice(observations, func(i, j int) bool {
		if observations[i].Protocol != observations[j].Protocol {
			return observations[i].Protocol < observations[j].Protocol
		}
		if observations[i].LocalDeviceID != observations[j].LocalDeviceID {
			return observations[i].LocalDeviceID.String() < observations[j].LocalDeviceID.String()
		}
		if observations[i].RemoteDeviceID != observations[j].RemoteDeviceID {
			return observations[i].RemoteDeviceID.String() < observations[j].RemoteDeviceID.String()
		}
		if observations[i].LocalPort != observations[j].LocalPort {
			return observations[i].LocalPort < observations[j].LocalPort
		}
		if observations[i].RemotePort != observations[j].RemotePort {
			return observations[i].RemotePort < observations[j].RemotePort
		}
		if observations[i].RemoteIdentity != observations[j].RemoteIdentity {
			return observations[i].RemoteIdentity < observations[j].RemoteIdentity
		}
		return observations[i].LastObservedAt.Before(observations[j].LastObservedAt)
	})

	for _, observation := range observations {
		if observation.LocalDeviceID == uuid.Nil || observation.RemoteDeviceID == uuid.Nil {
			continue
		}

		link := domain.Link{
			SourceDeviceID:    observation.LocalDeviceID,
			SourceIfName:      observation.LocalPort,
			TargetDeviceID:    observation.RemoteDeviceID,
			TargetIfName:      observation.RemotePort,
			DiscoveryProtocol: observation.Protocol,
		}

		key := string(link.DiscoveryProtocol) + "|" +
			link.SourceDeviceID.String() + "|" + link.SourceIfName + "|" +
			link.TargetDeviceID.String() + "|" + link.TargetIfName
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, link)
	}

	return candidates
}

func ApplyObservations(observations []Observation, writer LinkWriter) (ApplyResult, error) {
	result := ApplyResult{}
	for _, link := range CandidateLinks(observations) {
		linkCopy := link
		upsertResult, err := writer.UpsertDetailed(&linkCopy)
		if err != nil {
			return ApplyResult{}, err
		}
		if upsertResult.Created {
			result.LinksCreated++
		}
		if upsertResult.Changed {
			result.TopologyChanged = true
		}
		result.Events = append(result.Events, ApplyEvent{
			Link:   linkCopy,
			Result: upsertResult,
		})
	}
	return result, nil
}
