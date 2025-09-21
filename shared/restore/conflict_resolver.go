package restore

import (
	"reflect"
	"fmt"
	"strings"

	sharedconfig "shared-config/config"
	
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/runtime"
)

// ConflictResolver handles resource conflicts during restore operations
type ConflictResolver struct {
	config    *sharedconfig.SharedConfig
	mergeOptions MergeOptions
}

// MergeOptions defines how resources should be merged
type MergeOptions struct {
	PreserveLabels      []string          `json:"preserve_labels"`
	PreserveAnnotations []string          `json:"preserve_annotations"`
	IgnoreFields        []string          `json:"ignore_fields"`
	ForceFields         []string          `json:"force_fields"`
	CustomMergeRules    map[string]string `json:"custom_merge_rules"`
}

// MergeResult contains the result of a merge operation
type MergeResult struct {
	Merged      *unstructured.Unstructured `json:"merged"`
	Conflicts   []FieldConflict            `json:"conflicts"`
	Changes     []FieldChange              `json:"changes"`
	Strategy    string                     `json:"strategy"`
	Success     bool                       `json:"success"`
	Message     string                     `json:"message,omitempty"`
}

// FieldConflict represents a conflict between existing and desired resource fields
type FieldConflict struct {
	Field         string      `json:"field"`
	ExistingValue interface{} `json:"existing_value"`
	DesiredValue  interface{} `json:"desired_value"`
	Resolution    string      `json:"resolution"`
	Reason        string      `json:"reason"`
}

// FieldChange represents a change made during merge
type FieldChange struct {
	Field     string      `json:"field"`
	OldValue  interface{} `json:"old_value"`
	NewValue  interface{} `json:"new_value"`
	Action    string      `json:"action"` // added, modified, removed
}

// NewConflictResolver creates a new conflict resolver
func NewConflictResolver(config *sharedconfig.SharedConfig) *ConflictResolver {
	mergeOptions := MergeOptions{
		PreserveLabels: []string{
			"app.kubernetes.io/managed-by",
			"app.kubernetes.io/instance",
		},
		PreserveAnnotations: []string{
			"kubectl.kubernetes.io/last-applied-configuration",
			"deployment.kubernetes.io/revision",
		},
		IgnoreFields: []string{
			"metadata.resourceVersion",
			"metadata.uid",
			"metadata.generation",
			"metadata.creationTimestamp",
			"metadata.managedFields",
			"status",
		},
		ForceFields: []string{
			"spec",
		},
	}

	return &ConflictResolver{
		config:       config,
		mergeOptions: mergeOptions,
	}
}

// MergeResources merges existing and desired resources
func (cr *ConflictResolver) MergeResources(existing, desired *unstructured.Unstructured) *unstructured.Unstructured {
	result := cr.PerformMerge(existing, desired)
	return result.Merged
}

// PerformMerge performs detailed merge with conflict tracking
func (cr *ConflictResolver) PerformMerge(existing, desired *unstructured.Unstructured) *MergeResult {
	result := &MergeResult{
		Conflicts: make([]FieldConflict, 0),
		Changes:   make([]FieldChange, 0),
		Strategy:  "three_way_merge",
		Success:   true,
	}

	// Start with a copy of existing resource
	merged := existing.DeepCopy()

	// Apply merge rules based on resource type
	switch desired.GetKind() {
	case "Deployment":
		cr.mergeDeployment(merged, desired, result)
	case "Service":
		cr.mergeService(merged, desired, result)
	case "ConfigMap":
		cr.mergeConfigMap(merged, desired, result)
	case "Secret":
		cr.mergeSecret(merged, desired, result)
	case "Ingress":
		cr.mergeIngress(merged, desired, result)
	case "PersistentVolumeClaim":
		cr.mergePVC(merged, desired, result)
	default:
		cr.mergeGeneric(merged, desired, result)
	}

	result.Merged = merged
	return result
}

// mergeDeployment handles deployment-specific merge logic
func (cr *ConflictResolver) mergeDeployment(existing, desired *unstructured.Unstructured, result *MergeResult) {
	// Preserve certain metadata
	cr.preserveMetadata(existing, desired, result)

	// Merge spec strategically
	existingSpec, _, _ := unstructured.NestedMap(existing.Object, "spec")
	desiredSpec, _, _ := unstructured.NestedMap(desired.Object, "spec")

	if existingSpec != nil && desiredSpec != nil {
		// Special handling for deployment spec fields
		cr.mergeDeploymentSpec(existingSpec, desiredSpec, result)
		unstructured.SetNestedMap(existing.Object, existingSpec, "spec")
	}
}

// mergeDeploymentSpec handles deployment spec merge
func (cr *ConflictResolver) mergeDeploymentSpec(existing, desired map[string]interface{}, result *MergeResult) {
	// Handle replicas specially
	existingReplicas, existingReplicasOk := existing["replicas"]
	desiredReplicas, desiredReplicasOk := desired["replicas"]
	
	if existingReplicasOk && desiredReplicasOk {
		if !reflect.DeepEqual(existingReplicas, desiredReplicas) {
			conflict := FieldConflict{
				Field:         "spec.replicas",
				ExistingValue: existingReplicas,
				DesiredValue:  desiredReplicas,
				Resolution:    "keep_existing",
				Reason:        "Preserve current scaling state",
			}
			result.Conflicts = append(result.Conflicts, conflict)
			
			// Keep existing replicas unless explicitly forced
			change := FieldChange{
				Field:    "spec.replicas",
				OldValue: desiredReplicas,
				NewValue: existingReplicas,
				Action:   "preserved",
			}
			result.Changes = append(result.Changes, change)
		}
	}

	// Merge template
	existingTemplate, existingTemplateOk := existing["template"].(map[string]interface{})
	desiredTemplate, desiredTemplateOk := desired["template"].(map[string]interface{})
	
	if existingTemplateOk && desiredTemplateOk {
		cr.mergeMap(existingTemplate, desiredTemplate, "spec.template", result)
		existing["template"] = existingTemplate
	} else if desiredTemplateOk {
		existing["template"] = desiredTemplate
		change := FieldChange{
			Field:    "spec.template",
			OldValue: nil,
			NewValue: desiredTemplate,
			Action:   "added",
		}
		result.Changes = append(result.Changes, change)
	}

	// Merge other spec fields
	for key, value := range desired {
		if key != "replicas" && key != "template" {
			if existingValue, exists := existing[key]; exists {
				if !reflect.DeepEqual(existingValue, value) {
					existing[key] = value
					change := FieldChange{
						Field:    fmt.Sprintf("spec.%s", key),
						OldValue: existingValue,
						NewValue: value,
						Action:   "modified",
					}
					result.Changes = append(result.Changes, change)
				}
			} else {
				existing[key] = value
				change := FieldChange{
					Field:    fmt.Sprintf("spec.%s", key),
					OldValue: nil,
					NewValue: value,
					Action:   "added",
				}
				result.Changes = append(result.Changes, change)
			}
		}
	}
}

// mergeService handles service-specific merge logic
func (cr *ConflictResolver) mergeService(existing, desired *unstructured.Unstructured, result *MergeResult) {
	cr.preserveMetadata(existing, desired, result)

	// Special handling for service ports and selectors
	existingSpec, _, _ := unstructured.NestedMap(existing.Object, "spec")
	desiredSpec, _, _ := unstructured.NestedMap(desired.Object, "spec")

	if existingSpec != nil && desiredSpec != nil {
		// Preserve cluster IP and node port allocations
		if clusterIP, exists := existingSpec["clusterIP"]; exists && clusterIP != "" {
			desiredSpec["clusterIP"] = clusterIP
		}

		// Merge ports with special handling for nodePort
		cr.mergeServicePorts(existingSpec, desiredSpec, result)

		unstructured.SetNestedMap(existing.Object, desiredSpec, "spec")
	}
}

// mergeServicePorts handles service port merge logic
func (cr *ConflictResolver) mergeServicePorts(existing, desired map[string]interface{}, result *MergeResult) {
	existingPorts, existingPortsOk := existing["ports"].([]interface{})
	desiredPorts, desiredPortsOk := desired["ports"].([]interface{})

	if !existingPortsOk || !desiredPortsOk {
		return
	}

	// Create map of existing ports by name/port for easier lookup
	existingPortMap := make(map[string]map[string]interface{})
	for _, port := range existingPorts {
		if portMap, ok := port.(map[string]interface{}); ok {
			key := cr.getPortKey(portMap)
			existingPortMap[key] = portMap
		}
	}

	// Merge desired ports with existing nodePort allocations
	mergedPorts := make([]interface{}, 0)
	for _, port := range desiredPorts {
		if desiredPortMap, ok := port.(map[string]interface{}); ok {
			key := cr.getPortKey(desiredPortMap)
			
			if existingPortMap, exists := existingPortMap[key]; exists {
				// Preserve nodePort if it exists
				if nodePort, hasNodePort := existingPortMap["nodePort"]; hasNodePort {
					desiredPortMap["nodePort"] = nodePort
				}
			}
			
			mergedPorts = append(mergedPorts, desiredPortMap)
		}
	}

	desired["ports"] = mergedPorts
}

// getPortKey creates a unique key for service port identification
func (cr *ConflictResolver) getPortKey(portMap map[string]interface{}) string {
	name, _ := portMap["name"].(string)
	port, _ := portMap["port"].(int64)
	protocol, _ := portMap["protocol"].(string)
	
	if protocol == "" {
		protocol = "TCP"
	}
	
	if name != "" {
		return fmt.Sprintf("%s:%d:%s", name, port, protocol)
	}
	return fmt.Sprintf("%d:%s", port, protocol)
}

// mergeConfigMap handles ConfigMap merge logic
func (cr *ConflictResolver) mergeConfigMap(existing, desired *unstructured.Unstructured, result *MergeResult) {
	cr.preserveMetadata(existing, desired, result)

	// Merge data and binaryData
	cr.mergeConfigMapData(existing, desired, "data", result)
	cr.mergeConfigMapData(existing, desired, "binaryData", result)
}

// mergeConfigMapData merges ConfigMap data fields
func (cr *ConflictResolver) mergeConfigMapData(existing, desired *unstructured.Unstructured, field string, result *MergeResult) {
	existingData, existingOk, _ := unstructured.NestedStringMap(existing.Object, field)
	desiredData, desiredOk, _ := unstructured.NestedStringMap(desired.Object, field)

	if !desiredOk {
		return
	}

	if !existingOk {
		existingData = make(map[string]string)
	}

	// Merge data entries
	for key, value := range desiredData {
		if existingValue, exists := existingData[key]; exists {
			if existingValue != value {
				conflict := FieldConflict{
					Field:         fmt.Sprintf("%s.%s", field, key),
					ExistingValue: existingValue,
					DesiredValue:  value,
					Resolution:    "use_desired",
					Reason:        "Update with backup data",
				}
				result.Conflicts = append(result.Conflicts, conflict)

				change := FieldChange{
					Field:    fmt.Sprintf("%s.%s", field, key),
					OldValue: existingValue,
					NewValue: value,
					Action:   "modified",
				}
				result.Changes = append(result.Changes, change)
			}
		} else {
			change := FieldChange{
				Field:    fmt.Sprintf("%s.%s", field, key),
				OldValue: nil,
				NewValue: value,
				Action:   "added",
			}
			result.Changes = append(result.Changes, change)
		}
		existingData[key] = value
	}

	unstructured.SetNestedStringMap(existing.Object, existingData, field)
}

// mergeSecret handles Secret merge logic
func (cr *ConflictResolver) mergeSecret(existing, desired *unstructured.Unstructured, result *MergeResult) {
	cr.preserveMetadata(existing, desired, result)

	// Secrets are sensitive - be more careful about conflicts
	existingData, existingOk, _ := unstructured.NestedMap(existing.Object, "data")
	desiredData, desiredOk, _ := unstructured.NestedMap(desired.Object, "data")

	if !desiredOk {
		return
	}

	if !existingOk {
		existingData = make(map[string]interface{})
	}

	// For secrets, warn about conflicts but allow overwrites
	for key, value := range desiredData {
		if existingValue, exists := existingData[key]; exists {
			if !reflect.DeepEqual(existingValue, value) {
				conflict := FieldConflict{
					Field:         fmt.Sprintf("data.%s", key),
					ExistingValue: "[REDACTED]",
					DesiredValue:  "[REDACTED]",
					Resolution:    "overwrite",
					Reason:        "Secret data from backup",
				}
				result.Conflicts = append(result.Conflicts, conflict)

				change := FieldChange{
					Field:    fmt.Sprintf("data.%s", key),
					OldValue: "[REDACTED]",
					NewValue: "[REDACTED]",
					Action:   "modified",
				}
				result.Changes = append(result.Changes, change)
			}
		} else {
			change := FieldChange{
				Field:    fmt.Sprintf("data.%s", key),
				OldValue: nil,
				NewValue: "[REDACTED]",
				Action:   "added",
			}
			result.Changes = append(result.Changes, change)
		}
		existingData[key] = value
	}

	unstructured.SetNestedMap(existing.Object, existingData, "data")
}

// mergeIngress handles Ingress merge logic
func (cr *ConflictResolver) mergeIngress(existing, desired *unstructured.Unstructured, result *MergeResult) {
	cr.preserveMetadata(existing, desired, result)

	// Merge spec with special handling for rules and TLS
	existingSpec, _, _ := unstructured.NestedMap(existing.Object, "spec")
	desiredSpec, _, _ := unstructured.NestedMap(desired.Object, "spec")

	if existingSpec != nil && desiredSpec != nil {
		cr.mergeMap(existingSpec, desiredSpec, "spec", result)
		unstructured.SetNestedMap(existing.Object, existingSpec, "spec")
	}
}

// mergePVC handles PersistentVolumeClaim merge logic
func (cr *ConflictResolver) mergePVC(existing, desired *unstructured.Unstructured, result *MergeResult) {
	cr.preserveMetadata(existing, desired, result)

	// PVCs have immutable fields, so be careful
	existingSpec, _, _ := unstructured.NestedMap(existing.Object, "spec")
	desiredSpec, _, _ := unstructured.NestedMap(desired.Object, "spec")

	if existingSpec != nil && desiredSpec != nil {
		// Check for immutable field conflicts
		cr.checkPVCImmutableFields(existingSpec, desiredSpec, result)
	}
}

// checkPVCImmutableFields validates immutable PVC fields
func (cr *ConflictResolver) checkPVCImmutableFields(existing, desired map[string]interface{}, result *MergeResult) {
	immutableFields := []string{"accessModes", "storageClassName", "volumeMode"}

	for _, field := range immutableFields {
		existingValue := existing[field]
		desiredValue := desired[field]

		if existingValue != nil && desiredValue != nil && !reflect.DeepEqual(existingValue, desiredValue) {
			conflict := FieldConflict{
				Field:         fmt.Sprintf("spec.%s", field),
				ExistingValue: existingValue,
				DesiredValue:  desiredValue,
				Resolution:    "keep_existing",
				Reason:        "Field is immutable in PVC",
			}
			result.Conflicts = append(result.Conflicts, conflict)
			result.Success = false
		}
	}
}

// mergeGeneric handles generic resource merge
func (cr *ConflictResolver) mergeGeneric(existing, desired *unstructured.Unstructured, result *MergeResult) {
	cr.preserveMetadata(existing, desired, result)

	// Merge spec if it exists
	existingSpec, existingSpecOk, _ := unstructured.NestedMap(existing.Object, "spec")
	desiredSpec, desiredSpecOk, _ := unstructured.NestedMap(desired.Object, "spec")

	if existingSpecOk && desiredSpecOk {
		cr.mergeMap(existingSpec, desiredSpec, "spec", result)
		unstructured.SetNestedMap(existing.Object, existingSpec, "spec")
	} else if desiredSpecOk {
		unstructured.SetNestedMap(existing.Object, desiredSpec, "spec")
		change := FieldChange{
			Field:    "spec",
			OldValue: nil,
			NewValue: desiredSpec,
			Action:   "added",
		}
		result.Changes = append(result.Changes, change)
	}
}

// preserveMetadata preserves important metadata during merge
func (cr *ConflictResolver) preserveMetadata(existing, desired *unstructured.Unstructured, result *MergeResult) {
	// Preserve certain labels
	existingLabels := existing.GetLabels()
	desiredLabels := desired.GetLabels()
	
	if existingLabels == nil {
		existingLabels = make(map[string]string)
	}
	if desiredLabels == nil {
		desiredLabels = make(map[string]string)
	}

	mergedLabels := make(map[string]string)
	
	// Copy existing labels first
	for key, value := range existingLabels {
		mergedLabels[key] = value
	}

	// Add/update with desired labels
	for key, value := range desiredLabels {
		if shouldPreserveLabel(key, cr.mergeOptions.PreserveLabels) {
			if existingValue, exists := existingLabels[key]; exists && existingValue != value {
				conflict := FieldConflict{
					Field:         fmt.Sprintf("metadata.labels.%s", key),
					ExistingValue: existingValue,
					DesiredValue:  value,
					Resolution:    "keep_existing",
					Reason:        "Preserve existing managed label",
				}
				result.Conflicts = append(result.Conflicts, conflict)
			}
		} else {
			if existingValue, exists := mergedLabels[key]; exists && existingValue != value {
				change := FieldChange{
					Field:    fmt.Sprintf("metadata.labels.%s", key),
					OldValue: existingValue,
					NewValue: value,
					Action:   "modified",
				}
				result.Changes = append(result.Changes, change)
			} else if !exists {
				change := FieldChange{
					Field:    fmt.Sprintf("metadata.labels.%s", key),
					OldValue: nil,
					NewValue: value,
					Action:   "added",
				}
				result.Changes = append(result.Changes, change)
			}
			mergedLabels[key] = value
		}
	}

	existing.SetLabels(mergedLabels)

	// Similar logic for annotations
	cr.mergeAnnotations(existing, desired, result)
}

// mergeAnnotations merges annotations with preservation rules
func (cr *ConflictResolver) mergeAnnotations(existing, desired *unstructured.Unstructured, result *MergeResult) {
	existingAnnotations := existing.GetAnnotations()
	desiredAnnotations := desired.GetAnnotations()
	
	if existingAnnotations == nil {
		existingAnnotations = make(map[string]string)
	}
	if desiredAnnotations == nil {
		desiredAnnotations = make(map[string]string)
	}

	mergedAnnotations := make(map[string]string)
	
	// Copy existing annotations first
	for key, value := range existingAnnotations {
		mergedAnnotations[key] = value
	}

	// Add/update with desired annotations
	for key, value := range desiredAnnotations {
		if shouldPreserveAnnotation(key, cr.mergeOptions.PreserveAnnotations) {
			if existingValue, exists := existingAnnotations[key]; exists && existingValue != value {
				conflict := FieldConflict{
					Field:         fmt.Sprintf("metadata.annotations.%s", key),
					ExistingValue: existingValue,
					DesiredValue:  value,
					Resolution:    "keep_existing",
					Reason:        "Preserve existing managed annotation",
				}
				result.Conflicts = append(result.Conflicts, conflict)
			}
		} else {
			if existingValue, exists := mergedAnnotations[key]; exists && existingValue != value {
				change := FieldChange{
					Field:    fmt.Sprintf("metadata.annotations.%s", key),
					OldValue: existingValue,
					NewValue: value,
					Action:   "modified",
				}
				result.Changes = append(result.Changes, change)
			} else if !exists {
				change := FieldChange{
					Field:    fmt.Sprintf("metadata.annotations.%s", key),
					OldValue: nil,
					NewValue: value,
					Action:   "added",
				}
				result.Changes = append(result.Changes, change)
			}
			mergedAnnotations[key] = value
		}
	}

	existing.SetAnnotations(mergedAnnotations)
}

// mergeMap recursively merges two maps
func (cr *ConflictResolver) mergeMap(existing, desired map[string]interface{}, path string, result *MergeResult) {
	for key, value := range desired {
		fieldPath := fmt.Sprintf("%s.%s", path, key)
		
		if shouldIgnoreField(fieldPath, cr.mergeOptions.IgnoreFields) {
			continue
		}

		if existingValue, exists := existing[key]; exists {
			if reflect.DeepEqual(existingValue, value) {
				continue // No change needed
			}

			// Handle nested maps
			if existingMap, existingIsMap := existingValue.(map[string]interface{}); existingIsMap {
				if desiredMap, desiredIsMap := value.(map[string]interface{}); desiredIsMap {
					cr.mergeMap(existingMap, desiredMap, fieldPath, result)
					existing[key] = existingMap
					continue
				}
			}

			// Handle conflicts
			if shouldForceField(fieldPath, cr.mergeOptions.ForceFields) {
				existing[key] = value
				change := FieldChange{
					Field:    fieldPath,
					OldValue: existingValue,
					NewValue: value,
					Action:   "forced",
				}
				result.Changes = append(result.Changes, change)
			} else {
				conflict := FieldConflict{
					Field:         fieldPath,
					ExistingValue: existingValue,
					DesiredValue:  value,
					Resolution:    "use_desired",
					Reason:        "Default merge strategy",
				}
				result.Conflicts = append(result.Conflicts, conflict)

				existing[key] = value
				change := FieldChange{
					Field:    fieldPath,
					OldValue: existingValue,
					NewValue: value,
					Action:   "modified",
				}
				result.Changes = append(result.Changes, change)
			}
		} else {
			// New field
			existing[key] = value
			change := FieldChange{
				Field:    fieldPath,
				OldValue: nil,
				NewValue: value,
				Action:   "added",
			}
			result.Changes = append(result.Changes, change)
		}
	}
}

// Helper functions

func shouldPreserveLabel(label string, preserveList []string) bool {
	for _, preserve := range preserveList {
		if strings.Contains(label, preserve) {
			return true
		}
	}
	return false
}

func shouldPreserveAnnotation(annotation string, preserveList []string) bool {
	for _, preserve := range preserveList {
		if strings.Contains(annotation, preserve) {
			return true
		}
	}
	return false
}

func shouldIgnoreField(field string, ignoreList []string) bool {
	for _, ignore := range ignoreList {
		if strings.Contains(field, ignore) {
			return true
		}
	}
	return false
}

func shouldForceField(field string, forceList []string) bool {
	for _, force := range forceList {
		if strings.Contains(field, force) {
			return true
		}
	}
	return false
}