package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/logging"
)

var (
	// ErrDeviceImportInvalidConfiguration reports options that could not form a valid preview.
	ErrDeviceImportInvalidConfiguration = errors.New("invalid device import configuration")
	// ErrDeviceImportDigestMismatch reports that commit did not receive the previewed file bytes.
	ErrDeviceImportDigestMismatch = errors.New("device import digest mismatch")
	// ErrDeviceImportConfigurationChanged reports that a previewed destination or profile no longer exists.
	ErrDeviceImportConfigurationChanged = errors.New("device import configuration changed")
	errDeviceImportInvalidFile          = errors.New("invalid device import file")
)

// DeviceImportRequest carries one stateless preview or commit operation.
type DeviceImportRequest struct {
	FileBytes          []byte
	MetricsMode        DeviceImportMode
	SNMPProfileID      *uuid.UUID
	MapID              uuid.UUID
	AreaID             *uuid.UUID
	ExpectedFileDigest string
	Actor              DeviceImportActor
}

// DeviceImportActor identifies the administrator responsible for a committed import.
type DeviceImportActor struct {
	UserID    uuid.UUID
	IPAddress string
	UserAgent string
}

// DeviceImportConfiguration is the safe normalized configuration returned to clients.
type DeviceImportConfiguration struct {
	MetricsMode   DeviceImportMode `json:"metrics_mode"`
	SNMPProfileID *uuid.UUID       `json:"snmp_profile_id"`
	MapID         uuid.UUID        `json:"map_id"`
	AreaID        *uuid.UUID       `json:"area_id"`
}

// DeviceImportTargetStatus is a stable preview or commit classification.
type DeviceImportTargetStatus string

const (
	DeviceImportTargetStatusReady                  DeviceImportTargetStatus = "ready"
	DeviceImportTargetStatusInvalid                DeviceImportTargetStatus = "invalid"
	DeviceImportTargetStatusSkippedDuplicateInFile DeviceImportTargetStatus = "skipped_duplicate_in_file"
	DeviceImportTargetStatusSkippedExisting        DeviceImportTargetStatus = "skipped_existing"
	DeviceImportTargetStatusCreated                DeviceImportTargetStatus = "created"
	DeviceImportTargetStatusFailed                 DeviceImportTargetStatus = "failed"
	DeviceImportTargetStatusNotProcessed           DeviceImportTargetStatus = "not_processed"
)

// DeviceImportPreviewSummary contains target and group counts for a preview.
type DeviceImportPreviewSummary struct {
	Total                  int `json:"total"`
	Ready                  int `json:"ready"`
	Invalid                int `json:"invalid"`
	InvalidGroups          int `json:"invalid_groups"`
	SkippedExisting        int `json:"skipped_existing"`
	SkippedDuplicateInFile int `json:"skipped_duplicate_in_file"`
}

// DeviceImportCommitSummary contains final per-target counts for a commit.
type DeviceImportCommitSummary struct {
	Total        int `json:"total"`
	Created      int `json:"created"`
	Skipped      int `json:"skipped"`
	Failed       int `json:"failed"`
	NotProcessed int `json:"not_processed"`
}

// DeviceImportDiagnostic is a label-free group-level file-SD diagnostic.
type DeviceImportDiagnostic struct {
	GroupIndex int    `json:"group_index"`
	Message    string `json:"message"`
}

// DeviceImportPreviewTarget describes one ordered target without device credentials.
type DeviceImportPreviewTarget struct {
	GroupIndex int                      `json:"group_index"`
	ItemIndex  int                      `json:"item_index"`
	Target     string                   `json:"target"`
	Address    string                   `json:"address"`
	Status     DeviceImportTargetStatus `json:"status"`
	Message    string                   `json:"message,omitempty"`
}

// DeviceImportResult describes one ordered commit outcome.
type DeviceImportResult struct {
	GroupIndex int                      `json:"group_index"`
	ItemIndex  int                      `json:"item_index"`
	Target     string                   `json:"target"`
	Address    string                   `json:"address"`
	Status     DeviceImportTargetStatus `json:"status"`
	Message    string                   `json:"message,omitempty"`
	DeviceID   *uuid.UUID               `json:"device_id,omitempty"`
}

// DeviceImportPreview is a stateless, side-effect-free classification of uploaded bytes.
type DeviceImportPreview struct {
	FileDigest    string                      `json:"file_digest"`
	Configuration DeviceImportConfiguration   `json:"configuration"`
	Summary       DeviceImportPreviewSummary  `json:"summary"`
	Targets       []DeviceImportPreviewTarget `json:"targets"`
	Diagnostics   []DeviceImportDiagnostic    `json:"diagnostics"`
}

// DeviceImportCommitResult contains ordered outcomes, including partial systemic failures.
type DeviceImportCommitResult struct {
	FileDigest    string                    `json:"file_digest"`
	Configuration DeviceImportConfiguration `json:"configuration"`
	Summary       DeviceImportCommitSummary `json:"summary"`
	Results       []DeviceImportResult      `json:"results"`
	Diagnostics   []DeviceImportDiagnostic  `json:"diagnostics"`
	Incomplete    bool                      `json:"incomplete"`
}

// DeviceImportService orchestrates label-blind preview and one-time partial commit.
type DeviceImportService struct {
	store          domain.DeviceImportStore
	devices        *DeviceService
	maps           domain.CanvasMapRepository
	profiles       domain.SNMPProfileRepository
	auditLogs      domain.AuditLogRepository
	reprobeDevice  func(context.Context, uuid.UUID) error
	notifyTopology func()
	now            func() time.Time
}

type validatedDeviceImport struct {
	configuration DeviceImportConfiguration
	credentials   domain.SNMPCredentials
}

const (
	deviceImportDuplicateMessage    = "duplicate address in uploaded file"
	deviceImportExistingMessage     = "address already exists"
	deviceImportFailedMessage       = "device could not be imported"
	deviceImportNotProcessedMessage = "not processed because the import stopped"
)

// NewDeviceImportService creates the stateless import orchestrator.
func NewDeviceImportService(
	store domain.DeviceImportStore,
	devices *DeviceService,
	maps domain.CanvasMapRepository,
	profiles domain.SNMPProfileRepository,
	auditLogs domain.AuditLogRepository,
) *DeviceImportService {
	service := &DeviceImportService{
		store:     store,
		devices:   devices,
		maps:      maps,
		profiles:  profiles,
		auditLogs: auditLogs,
		now:       time.Now,
	}
	if devices != nil {
		service.reprobeDevice = devices.ReprobeDevice
		service.notifyTopology = devices.NotifyTopologyChanged
	}
	return service
}

// Preview validates configuration and classifies targets without persistence or network probes.
func (s *DeviceImportService) Preview(
	ctx context.Context,
	request DeviceImportRequest,
) (DeviceImportPreview, error) {
	ctx = normalizeDeviceImportContext(ctx)
	preview := DeviceImportPreview{
		FileDigest:    computeDeviceImportDigest(request.FileBytes),
		Configuration: normalizeDeviceImportConfiguration(request),
		Targets:       []DeviceImportPreviewTarget{},
		Diagnostics:   []DeviceImportDiagnostic{},
	}

	validated, err := s.validateRequest(ctx, request, false)
	if err != nil {
		return preview, err
	}
	preview.Configuration = validated.configuration
	parsed, err := ParsePrometheusFileSD(request.FileBytes, request.MetricsMode)
	if err != nil {
		return preview, sanitizeDeviceImportParserError(err)
	}
	preview.Diagnostics = importDiagnostics(parsed.Diagnostics)
	preview.Summary.InvalidGroups = len(preview.Diagnostics)
	preview.Summary.Total = len(parsed.Targets)

	candidates := make([]string, 0, len(parsed.Targets))
	for _, target := range parsed.Targets {
		if target.ValidationError == "" && target.DuplicateOf == nil {
			candidates = append(candidates, target.CanonicalHost)
		}
	}
	if s == nil || s.store == nil {
		return preview, domain.ErrDeviceImportStoreUnavailable
	}
	existing, err := s.store.ExistingCanonicalAddresses(ctx, candidates)
	if err != nil {
		return preview, s.sanitizeDeviceImportDependencyError(
			err,
			"existing-address lookup",
			preview.FileDigest,
			preview.Configuration,
		)
	}

	for _, target := range parsed.Targets {
		row := newDeviceImportPreviewTarget(target)
		switch {
		case target.ValidationError != "":
			row.Status = DeviceImportTargetStatusInvalid
			row.Message = target.ValidationError
			preview.Summary.Invalid++
		case target.DuplicateOf != nil:
			row.Status = DeviceImportTargetStatusSkippedDuplicateInFile
			row.Message = deviceImportDuplicateMessage
			preview.Summary.SkippedDuplicateInFile++
		default:
			if _, buildErr := s.buildDeviceDraft(target, validated); buildErr != nil {
				row.Status = DeviceImportTargetStatusInvalid
				row.Message = deviceImportFailedMessage
				preview.Summary.Invalid++
			} else if _, found := existing[target.CanonicalHost]; found {
				row.Status = DeviceImportTargetStatusSkippedExisting
				row.Message = deviceImportExistingMessage
				preview.Summary.SkippedExisting++
			} else {
				row.Status = DeviceImportTargetStatusReady
				preview.Summary.Ready++
			}
		}
		preview.Targets = append(preview.Targets, row)
	}
	return preview, nil
}

// Commit revalidates exact bytes and creates each candidate in its own atomic transaction.
func (s *DeviceImportService) Commit(
	ctx context.Context,
	request DeviceImportRequest,
) (DeviceImportCommitResult, error) {
	ctx = normalizeDeviceImportContext(ctx)
	digest := computeDeviceImportDigest(request.FileBytes)
	result := DeviceImportCommitResult{
		FileDigest:    digest,
		Configuration: normalizeDeviceImportConfiguration(request),
		Results:       []DeviceImportResult{},
		Diagnostics:   []DeviceImportDiagnostic{},
	}
	if request.ExpectedFileDigest == "" || request.ExpectedFileDigest != digest {
		return result, ErrDeviceImportDigestMismatch
	}

	validated, err := s.validateRequest(ctx, request, true)
	if err != nil {
		return result, err
	}
	result.Configuration = validated.configuration
	parsed, err := ParsePrometheusFileSD(request.FileBytes, request.MetricsMode)
	if err != nil {
		return result, sanitizeDeviceImportParserError(err)
	}
	result.Diagnostics = importDiagnostics(parsed.Diagnostics)
	result.Summary.Total = len(parsed.Targets)

	createdIDs := make([]uuid.UUID, 0, len(parsed.Targets))
	probeIDs := make([]uuid.UUID, 0, len(parsed.Targets))
	var abortErr error
	for _, target := range parsed.Targets {
		row := newDeviceImportResult(target)
		if abortErr != nil {
			row.Status = DeviceImportTargetStatusNotProcessed
			row.Message = deviceImportNotProcessedMessage
			result.Summary.NotProcessed++
			result.Results = append(result.Results, row)
			continue
		}
		if err := ctx.Err(); err != nil {
			abortErr = err
			result.Incomplete = true
			row.Status = DeviceImportTargetStatusNotProcessed
			row.Message = deviceImportNotProcessedMessage
			result.Summary.NotProcessed++
			result.Results = append(result.Results, row)
			continue
		}

		switch {
		case target.ValidationError != "":
			row.Status = DeviceImportTargetStatusInvalid
			row.Message = target.ValidationError
			result.Summary.Failed++
		case target.DuplicateOf != nil:
			row.Status = DeviceImportTargetStatusSkippedDuplicateInFile
			row.Message = deviceImportDuplicateMessage
			result.Summary.Skipped++
		default:
			device, buildErr := s.buildDeviceDraft(target, validated)
			if buildErr != nil {
				row.Status = DeviceImportTargetStatusFailed
				row.Message = deviceImportFailedMessage
				result.Summary.Failed++
				s.logTargetFailure(result.FileDigest, result.Configuration, target)
				break
			}
			if s == nil || s.store == nil {
				buildErr = domain.ErrDeviceImportStoreUnavailable
			} else {
				buildErr = s.store.CreateDeviceInMap(ctx, device, domain.DeviceImportPlacement{
					MapID:  validated.configuration.MapID,
					AreaID: cloneUUIDPointer(validated.configuration.AreaID),
				})
			}
			switch {
			case buildErr == nil:
				row.Status = DeviceImportTargetStatusCreated
				deviceID := device.ID
				row.DeviceID = &deviceID
				result.Summary.Created++
				createdIDs = append(createdIDs, device.ID)
				if deviceImportModeUsesSNMP(request.MetricsMode) {
					probeIDs = append(probeIDs, device.ID)
				}
			case errors.Is(buildErr, domain.ErrDeviceImportAddressConflict):
				row.Status = DeviceImportTargetStatusSkippedExisting
				row.Message = deviceImportExistingMessage
				result.Summary.Skipped++
			case errors.Is(buildErr, domain.ErrDeviceImportDestinationChanged):
				row.Status = DeviceImportTargetStatusNotProcessed
				row.Message = deviceImportNotProcessedMessage
				result.Summary.NotProcessed++
				result.Incomplete = true
				abortErr = ErrDeviceImportConfigurationChanged
				s.logTargetFailure(result.FileDigest, result.Configuration, target)
			case errors.Is(buildErr, domain.ErrDeviceImportStoreUnavailable):
				row.Status = DeviceImportTargetStatusNotProcessed
				row.Message = deviceImportNotProcessedMessage
				result.Summary.NotProcessed++
				result.Incomplete = true
				abortErr = domain.ErrDeviceImportStoreUnavailable
				s.logTargetFailure(result.FileDigest, result.Configuration, target)
			case errors.Is(buildErr, context.Canceled), errors.Is(buildErr, context.DeadlineExceeded):
				row.Status = DeviceImportTargetStatusNotProcessed
				row.Message = deviceImportNotProcessedMessage
				result.Summary.NotProcessed++
				result.Incomplete = true
				abortErr = exactDeviceImportContextError(buildErr)
				s.logTargetFailure(result.FileDigest, result.Configuration, target)
			default:
				row.Status = DeviceImportTargetStatusFailed
				row.Message = deviceImportFailedMessage
				result.Summary.Failed++
				s.logTargetFailure(result.FileDigest, result.Configuration, target)
			}
		}
		result.Results = append(result.Results, row)
	}

	s.finishCommit(ctx, request, &result, createdIDs, probeIDs)
	return result, abortErr
}

func (s *DeviceImportService) validateRequest(
	ctx context.Context,
	request DeviceImportRequest,
	commit bool,
) (validatedDeviceImport, error) {
	validated := validatedDeviceImport{configuration: normalizeDeviceImportConfiguration(request)}
	if err := ctx.Err(); err != nil {
		return validated, err
	}
	if !deviceImportModeIsValid(request.MetricsMode) {
		return validated, invalidDeviceImportConfiguration("unsupported metrics mode")
	}
	usesSNMP := deviceImportModeUsesSNMP(request.MetricsMode)
	if request.MetricsMode == DeviceImportModePrometheus && request.SNMPProfileID != nil {
		return validated, invalidDeviceImportConfiguration("SNMP Profile is not allowed in Prometheus mode")
	}
	if usesSNMP && (request.SNMPProfileID == nil || *request.SNMPProfileID == uuid.Nil) {
		return validated, invalidDeviceImportConfiguration("SNMP Profile is required for the selected mode")
	}
	if request.MapID == uuid.Nil {
		return validated, invalidDeviceImportConfiguration("destination map is required")
	}
	if request.AreaID != nil && *request.AreaID == uuid.Nil {
		return validated, invalidDeviceImportConfiguration("selected area is invalid")
	}
	if s == nil || s.store == nil || s.devices == nil || s.maps == nil || (usesSNMP && s.profiles == nil) {
		return validated, domain.ErrDeviceImportStoreUnavailable
	}

	canvasMap, err := s.maps.GetByID(request.MapID)
	if err != nil {
		return validated, s.validateDeviceImportDependencyError(
			err,
			commit,
			"destination map",
			"canvas map not found",
			request,
		)
	}
	if canvasMap.ID != request.MapID {
		return validated, changedOrInvalidDeviceImportConfiguration(commit, "destination map is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return validated, err
	}
	membership, err := s.maps.GetMembership(request.MapID)
	if err != nil {
		return validated, s.validateDeviceImportDependencyError(
			err,
			commit,
			"destination map membership",
			"canvas map not found",
			request,
		)
	}
	if request.AreaID != nil && !deviceImportMembershipContainsArea(membership, *request.AreaID) {
		return validated, changedOrInvalidDeviceImportConfiguration(commit, "selected area is unavailable in the destination map")
	}

	if usesSNMP {
		if err := ctx.Err(); err != nil {
			return validated, err
		}
		profile, err := s.profiles.GetByID(*request.SNMPProfileID)
		if err != nil {
			return validated, s.validateDeviceImportDependencyError(
				err,
				commit,
				"selected SNMP Profile",
				"snmp profile not found",
				request,
			)
		}
		if profile == nil || profile.ID != *request.SNMPProfileID {
			return validated, changedOrInvalidDeviceImportConfiguration(commit, "selected SNMP Profile is unavailable")
		}
		validated.credentials = cloneDeviceImportCredentials(profile.Credentials)
	}
	return validated, nil
}

func (s *DeviceImportService) buildDeviceDraft(
	target ParsedDeviceImportTarget,
	validated validatedDeviceImport,
) (*domain.Device, error) {
	if s == nil || s.devices == nil {
		return nil, domain.ErrDeviceImportStoreUnavailable
	}
	mode := validated.configuration.MetricsMode
	metricsSource := domain.MetricsSourcePrometheus
	labelName := "instance"
	labelValue := target.RawTarget
	credentials := domain.SNMPCredentials{}
	switch mode {
	case DeviceImportModePrometheus:
		// Defaults already describe the Prometheus-only device.
	case DeviceImportModePrometheusFallback:
		metricsSource = domain.MetricsSourcePrometheusSNMPFallback
		credentials = cloneDeviceImportCredentials(validated.credentials)
	case DeviceImportModeSNMP:
		metricsSource = domain.MetricsSourceSNMP
		credentials = cloneDeviceImportCredentials(validated.credentials)
		labelName = ""
		labelValue = ""
	default:
		return nil, invalidDeviceImportConfiguration("unsupported metrics mode")
	}

	draft, err := s.devices.BuildDeviceDraft(DeviceDraftInput{
		IP:                   target.CanonicalHost,
		Hostname:             "",
		DeviceType:           domain.DeviceTypeUnknown,
		SNMPCredentials:      credentials,
		Tags:                 map[string]string{},
		Vendor:               "default",
		MetricsSource:        metricsSource,
		PrometheusLabelName:  labelName,
		PrometheusLabelValue: labelValue,
		AreaIDs:              nil,
		Addresses: []domain.DeviceAddress{{
			Address:   target.CanonicalHost,
			Role:      domain.DeviceAddressRolePrimary,
			IsPrimary: true,
		}},
	})
	if err != nil {
		return nil, err
	}
	// BuildDeviceDraft preserves legacy Prometheus defaults for empty selectors;
	// pure SNMP explicitly opts out after construction.
	if mode == DeviceImportModeSNMP {
		draft.PrometheusLabelName = ""
		draft.PrometheusLabelValue = ""
	}
	draft.AreaIDs = nil
	return draft, nil
}

func (s *DeviceImportService) finishCommit(
	ctx context.Context,
	request DeviceImportRequest,
	result *DeviceImportCommitResult,
	createdIDs []uuid.UUID,
	probeIDs []uuid.UUID,
) {
	if s == nil || result == nil {
		return
	}
	if s.store != nil {
		s.store.PublishCreatedDevices(createdIDs)
	}
	postCommitContext := context.WithoutCancel(normalizeDeviceImportContext(ctx))
	for _, deviceID := range probeIDs {
		if s.reprobeDevice == nil {
			continue
		}
		if err := s.reprobeDevice(postCommitContext, deviceID); err != nil {
			s.logPostCommitFailure("probe", result.FileDigest, result.Configuration)
		}
	}
	if len(createdIDs) > 0 && s.notifyTopology != nil {
		s.notifyTopology()
	}
	if err := s.appendAudit(postCommitContext, request, *result); err != nil {
		s.logPostCommitFailure("audit", result.FileDigest, result.Configuration)
	}
}

func (s *DeviceImportService) appendAudit(
	ctx context.Context,
	request DeviceImportRequest,
	result DeviceImportCommitResult,
) error {
	if s.auditLogs == nil {
		return nil
	}
	metadata := struct {
		ActorUserID   string                    `json:"actor_user_id"`
		FileDigest    string                    `json:"file_digest"`
		MetricsMode   DeviceImportMode          `json:"metrics_mode"`
		MapID         string                    `json:"map_id"`
		AreaID        *string                   `json:"area_id,omitempty"`
		SNMPProfileID *string                   `json:"snmp_profile_id,omitempty"`
		Summary       DeviceImportCommitSummary `json:"summary"`
	}{
		ActorUserID:   request.Actor.UserID.String(),
		FileDigest:    result.FileDigest,
		MetricsMode:   result.Configuration.MetricsMode,
		MapID:         result.Configuration.MapID.String(),
		AreaID:        uuidStringPointer(result.Configuration.AreaID),
		SNMPProfileID: uuidStringPointer(result.Configuration.SNMPProfileID),
		Summary:       result.Summary,
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	var actorID *uuid.UUID
	if request.Actor.UserID != uuid.Nil {
		value := request.Actor.UserID
		actorID = &value
	}
	now := time.Now().UTC()
	if s.now != nil {
		now = s.now().UTC()
	}
	entry := &domain.AuditLog{
		ID:           uuid.New(),
		ActorUserID:  actorID,
		Action:       "admin.device_import_completed",
		Resource:     "device_import",
		ResourceID:   result.Configuration.MapID.String(),
		MetadataJSON: string(encoded),
		IPAddress:    request.Actor.IPAddress,
		UserAgent:    request.Actor.UserAgent,
		CreatedAt:    now,
	}
	return s.auditLogs.AppendAuditLog(ctx, entry)
}

func (s *DeviceImportService) logTargetFailure(
	digest string,
	configuration DeviceImportConfiguration,
	target ParsedDeviceImportTarget,
) {
	logging.Errorf(
		"device import target failed reference=%s digest=%s mode=%s map_id=%s group=%d item=%d",
		uuid.NewString(),
		digest,
		configuration.MetricsMode,
		configuration.MapID,
		target.GroupIndex,
		target.ItemIndex,
	)
}

func (s *DeviceImportService) logPostCommitFailure(
	stage string,
	digest string,
	configuration DeviceImportConfiguration,
) {
	logging.Errorf(
		"device import post-commit stage failed reference=%s stage=%s digest=%s mode=%s map_id=%s",
		uuid.NewString(),
		stage,
		digest,
		configuration.MetricsMode,
		configuration.MapID,
	)
}

func newDeviceImportPreviewTarget(target ParsedDeviceImportTarget) DeviceImportPreviewTarget {
	return DeviceImportPreviewTarget{
		GroupIndex: target.GroupIndex,
		ItemIndex:  target.ItemIndex,
		Target:     target.RawTarget,
		Address:    target.CanonicalHost,
	}
}

func newDeviceImportResult(target ParsedDeviceImportTarget) DeviceImportResult {
	return DeviceImportResult{
		GroupIndex: target.GroupIndex,
		ItemIndex:  target.ItemIndex,
		Target:     target.RawTarget,
		Address:    target.CanonicalHost,
	}
}

func importDiagnostics(source []DeviceImportGroupDiagnostic) []DeviceImportDiagnostic {
	result := make([]DeviceImportDiagnostic, 0, len(source))
	for _, diagnostic := range source {
		result = append(result, DeviceImportDiagnostic{
			GroupIndex: diagnostic.GroupIndex,
			Message:    diagnostic.Message,
		})
	}
	return result
}

func computeDeviceImportDigest(fileBytes []byte) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256(fileBytes))
}

func normalizeDeviceImportConfiguration(request DeviceImportRequest) DeviceImportConfiguration {
	return DeviceImportConfiguration{
		MetricsMode:   request.MetricsMode,
		SNMPProfileID: cloneUUIDPointer(request.SNMPProfileID),
		MapID:         request.MapID,
		AreaID:        cloneUUIDPointer(request.AreaID),
	}
}

func normalizeDeviceImportContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func invalidDeviceImportConfiguration(message string) error {
	return fmt.Errorf("%w: %s", ErrDeviceImportInvalidConfiguration, message)
}

func changedOrInvalidDeviceImportConfiguration(commit bool, message string) error {
	if commit {
		return fmt.Errorf("%w: %s", ErrDeviceImportConfigurationChanged, message)
	}
	return invalidDeviceImportConfiguration(message)
}

func sanitizeDeviceImportParserError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrDeviceImportLimitExceeded) {
		return ErrDeviceImportLimitExceeded
	}
	if contextErr := exactDeviceImportContextError(err); contextErr != nil {
		return contextErr
	}
	return errDeviceImportInvalidFile
}

func (s *DeviceImportService) validateDeviceImportDependencyError(
	err error,
	commit bool,
	stage string,
	notFoundPhrase string,
	request DeviceImportRequest,
) error {
	if err == nil {
		return nil
	}
	if systemicErr := knownDeviceImportSystemicError(err); systemicErr != nil {
		return systemicErr
	}
	if notFoundPhrase != "" && strings.Contains(strings.ToLower(err.Error()), notFoundPhrase) {
		return changedOrInvalidDeviceImportConfiguration(commit, stage+" is unavailable")
	}
	return s.sanitizeDeviceImportDependencyError(
		err,
		stage,
		computeDeviceImportDigest(request.FileBytes),
		normalizeDeviceImportConfiguration(request),
	)
}

func (s *DeviceImportService) sanitizeDeviceImportDependencyError(
	err error,
	stage string,
	digest string,
	configuration DeviceImportConfiguration,
) error {
	if err == nil {
		return nil
	}
	if systemicErr := knownDeviceImportSystemicError(err); systemicErr != nil {
		return systemicErr
	}
	logging.Errorf(
		"device import dependency failed reference=%s stage=%s digest=%s mode=%s map_id=%s",
		uuid.NewString(),
		stage,
		digest,
		configuration.MetricsMode,
		configuration.MapID,
	)
	return domain.ErrDeviceImportStoreUnavailable
}

func knownDeviceImportSystemicError(err error) error {
	if contextErr := exactDeviceImportContextError(err); contextErr != nil {
		return contextErr
	}
	if errors.Is(err, domain.ErrDeviceImportStoreUnavailable) {
		return domain.ErrDeviceImportStoreUnavailable
	}
	return nil
}

func exactDeviceImportContextError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	default:
		return nil
	}
}

func deviceImportModeIsValid(mode DeviceImportMode) bool {
	return mode == DeviceImportModePrometheus ||
		mode == DeviceImportModePrometheusFallback ||
		mode == DeviceImportModeSNMP
}

func deviceImportModeUsesSNMP(mode DeviceImportMode) bool {
	return mode == DeviceImportModePrometheusFallback || mode == DeviceImportModeSNMP
}

func deviceImportMembershipContainsArea(membership domain.CanvasMapMembership, areaID uuid.UUID) bool {
	for _, area := range membership.Areas {
		if area.AreaID == areaID {
			return true
		}
	}
	return false
}

func cloneDeviceImportCredentials(credentials domain.SNMPCredentials) domain.SNMPCredentials {
	cloned := credentials
	if credentials.V2c != nil {
		value := *credentials.V2c
		cloned.V2c = &value
	}
	if credentials.V3 != nil {
		value := *credentials.V3
		cloned.V3 = &value
	}
	return cloned
}

func cloneUUIDPointer(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func uuidStringPointer(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}
