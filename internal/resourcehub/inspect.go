package resourcehub

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"sort"
	"strings"
)

// Inspect builds a read-only, canonical view of managed resources and their
// installations. It never mutates targets and does not expose filesystem paths.
func (m Manager) Inspect() (SyncInspection, error) {
	registry, err := m.LoadRegistry()
	if err != nil {
		return SyncInspection{}, err
	}
	inspection := SyncInspection{GeneratedAt: m.now(), Counts: SyncInspectionCounts{Managed: len(registry.Resources)}}

	resourceIDs := make([]string, 0, len(registry.Resources))
	for id := range registry.Resources {
		resourceIDs = append(resourceIDs, id)
	}
	sort.Strings(resourceIDs)

	exactGroups := map[string][]Resource{}
	logicalGroups := map[string][]Resource{}
	for _, id := range resourceIDs {
		resource := registry.Resources[id]
		exactGroups[exactResourceKey(resource)] = append(exactGroups[exactResourceKey(resource)], resource)
		logicalGroups[logicalResourceKey(resource)] = append(logicalGroups[logicalResourceKey(resource)], resource)
		if resourceContained(resource) {
			inspection.Counts.Contained++
		}
	}

	targetIDs := make([]string, 0, len(registry.Targets))
	for id := range registry.Targets {
		targetIDs = append(targetIDs, id)
	}
	sort.Strings(targetIDs)
	countedDestinations := map[string]struct{}{}

	exactKeys := make([]string, 0, len(exactGroups))
	for key := range exactGroups {
		exactKeys = append(exactKeys, key)
	}
	sort.Strings(exactKeys)
	for _, key := range exactKeys {
		group := exactGroups[key]
		sort.Slice(group, func(i, j int) bool { return group[i].ID < group[j].ID })
		primary := group[0]
		item := CanonicalResource{
			Identity: canonicalResourceIdentity(primary), Kind: primary.Kind,
			Namespace: resourceNamespace(primary), Name: primary.Name,
			Version: resourceVersion(primary), Digest: primary.Digest,
			Contained: resourceContained(primary),
		}
		for _, resource := range group {
			item.ResourceIDs = append(item.ResourceIDs, resource.ID)
		}
		for _, targetID := range targetIDs {
			target := registry.Targets[targetID]
			if !target.Enabled || !resourceTargetsAgent(primary, target.Agent) {
				continue
			}
			destination, destinationErr := targetDestination(target, primary)
			if destinationErr != nil {
				continue
			}
			installation := ResourceInstallation{
				TargetID: target.ID, Agent: target.Agent, Scope: target.Scope,
				DesiredDigest: primary.Digest, State: InstallationMissing,
				Message: "Not installed in this target.",
			}
			managed, _ := m.loadManagedState(target.ID)
			entry, managedEntryExists := managed.Entries[destination]
			installation.Managed = managedEntryExists && entry.ResourceID != ""
			current, digestErr := treeDigest(destination)
			switch {
			case errors.Is(digestErr, os.ErrNotExist):
				// missing is already the safe default
			case digestErr != nil:
				installation.State = InstallationConflict
				installation.Message = "The installation could not be inspected safely."
			case current == primary.Digest:
				installation.State = InstallationInSync
				installation.CurrentDigest = current
				installation.Message = "Installed content matches the approved source."
			case current != "":
				installation.State = InstallationDrifted
				installation.CurrentDigest = current
				installation.Message = "Installed content differs from the approved source."
			}
			item.Installations = append(item.Installations, installation)
			if installation.State != InstallationMissing {
				if _, counted := countedDestinations[destination]; !counted {
					countedDestinations[destination] = struct{}{}
					inspection.Counts.Installed++
					switch installation.State {
					case InstallationInSync:
						inspection.Counts.InSync++
					case InstallationDrifted:
						inspection.Counts.Drifted++
					case InstallationConflict:
						inspection.Counts.Conflicts++
					}
				}
			}
		}
		if len(group) > 1 {
			ids := append([]string(nil), item.ResourceIDs...)
			inspection.Duplicates = append(inspection.Duplicates, DuplicateGroup{
				Class: DuplicateExact, Key: key, ResourceIDs: ids,
				Message: "Multiple registry entries have the same canonical identity and content.", Review: true,
			})
		}
		if len(item.Installations) > 1 {
			targets := make([]string, 0, len(item.Installations))
			for _, installation := range item.Installations {
				if installation.State != InstallationMissing {
					targets = append(targets, installation.TargetID)
				}
			}
			if len(targets) > 1 {
				inspection.Duplicates = append(inspection.Duplicates, DuplicateGroup{
					Class: DuplicateEquivalent, Key: item.Identity, ResourceIDs: append([]string(nil), item.ResourceIDs...), TargetIDs: targets,
					Message: "One canonical resource is rendered into multiple connected targets.", Review: false,
				})
			}
		}
		inspection.Resources = append(inspection.Resources, item)
	}

	logicalKeys := make([]string, 0, len(logicalGroups))
	for key := range logicalGroups {
		logicalKeys = append(logicalKeys, key)
	}
	sort.Strings(logicalKeys)
	for _, key := range logicalKeys {
		group := logicalGroups[key]
		versions := map[string]struct{}{}
		digestsByVersion := map[string]map[string]struct{}{}
		ids := make([]string, 0, len(group))
		for _, resource := range group {
			version := resourceVersion(resource)
			versions[version] = struct{}{}
			if digestsByVersion[version] == nil {
				digestsByVersion[version] = map[string]struct{}{}
			}
			digestsByVersion[version][resource.Digest] = struct{}{}
			ids = append(ids, resource.ID)
		}
		sort.Strings(ids)
		if len(versions) > 1 {
			inspection.Duplicates = append(inspection.Duplicates, DuplicateGroup{
				Class: DuplicateVersion, Key: key, ResourceIDs: ids,
				Message: "Several versions of the same named resource are registered.", Review: true,
			})
		}
		for version, digests := range digestsByVersion {
			if len(digests) <= 1 {
				continue
			}
			inspection.Duplicates = append(inspection.Duplicates, DuplicateGroup{
				Class: DuplicateCollision, Key: key + "|" + version, ResourceIDs: ids,
				Message: "The same resource name and version has divergent content; no automatic winner was selected.", Review: true,
			})
			inspection.Counts.Conflicts++
		}
	}

	for _, targetID := range targetIDs {
		state, stateErr := m.loadManagedState(targetID)
		if stateErr != nil {
			return SyncInspection{}, stateErr
		}
		for _, entry := range state.Entries {
			if _, ok := registry.Resources[entry.ResourceID]; ok {
				continue
			}
			inspection.Duplicates = append(inspection.Duplicates, DuplicateGroup{
				Class: DuplicateOrphan, Key: targetID + "|" + entry.ResourceID,
				ResourceIDs: []string{entry.ResourceID}, TargetIDs: []string{targetID},
				Message: "Target ownership metadata references a resource that is no longer registered.", Review: true,
			})
			inspection.Counts.Orphans++
		}
	}

	sort.Slice(inspection.Resources, func(i, j int) bool {
		if strings.EqualFold(inspection.Resources[i].Name, inspection.Resources[j].Name) {
			return inspection.Resources[i].Identity < inspection.Resources[j].Identity
		}
		return strings.ToLower(inspection.Resources[i].Name) < strings.ToLower(inspection.Resources[j].Name)
	})
	sort.Slice(inspection.Duplicates, func(i, j int) bool {
		if inspection.Duplicates[i].Class != inspection.Duplicates[j].Class {
			return inspection.Duplicates[i].Class < inspection.Duplicates[j].Class
		}
		return inspection.Duplicates[i].Key < inspection.Duplicates[j].Key
	})
	inspection.Counts.Duplicates = len(inspection.Duplicates)
	return inspection, nil
}

func resourceNamespace(resource Resource) string {
	if value := strings.TrimSpace(resource.Metadata["namespace"]); value != "" {
		return strings.ToLower(value)
	}
	if value := strings.TrimSpace(resource.Source); value != "" {
		return "source"
	}
	return "local"
}

func resourceVersion(resource Resource) string {
	if value := strings.TrimSpace(resource.Metadata["version"]); value != "" {
		return value
	}
	return "unversioned"
}

func resourceContained(resource Resource) bool {
	if strings.TrimSpace(resource.Metadata["container"]) != "" || strings.TrimSpace(resource.Metadata["stack"]) != "" {
		return true
	}
	for _, tag := range resource.Tags {
		value := strings.ToLower(strings.TrimSpace(tag))
		if value == "contained" || strings.HasPrefix(value, "stack:") {
			return true
		}
	}
	return false
}

func logicalResourceKey(resource Resource) string {
	return strings.Join([]string{string(resource.Kind), resourceNamespace(resource), strings.ToLower(strings.TrimSpace(resource.Name))}, "|")
}

func exactResourceKey(resource Resource) string {
	return strings.Join([]string{logicalResourceKey(resource), resourceVersion(resource), resource.Digest}, "|")
}

func canonicalResourceIdentity(resource Resource) string {
	sum := sha256.Sum256([]byte(exactResourceKey(resource)))
	return "resource:" + hex.EncodeToString(sum[:])
}

func resourceTargetsAgent(resource Resource, agent Agent) bool {
	if len(resource.Targets) == 0 {
		return true
	}
	for _, target := range resource.Targets {
		if target == agent {
			return true
		}
	}
	return false
}
