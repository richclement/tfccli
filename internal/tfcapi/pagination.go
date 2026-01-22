package tfcapi

import (
	"context"

	tfe "github.com/hashicorp/go-tfe"
)

// DefaultPageSize is the default number of items per page.
const DefaultPageSize = 100

// CollectAllOrganizations fetches all pages of organizations.
func CollectAllOrganizations(
	ctx context.Context,
	client *tfe.Client,
	opts *tfe.OrganizationListOptions,
) ([]*tfe.Organization, error) {
	if opts == nil {
		opts = &tfe.OrganizationListOptions{}
	}
	if opts.PageSize == 0 {
		opts.PageSize = DefaultPageSize
	}
	opts.PageNumber = 1

	var all []*tfe.Organization
	for {
		list, err := client.Organizations.List(ctx, opts)
		if err != nil {
			return nil, err
		}
		all = append(all, list.Items...)

		// Stop if empty page or no more pages
		if len(list.Items) == 0 || list.Pagination == nil || list.Pagination.NextPage == 0 {
			break
		}
		opts.PageNumber = list.Pagination.NextPage
	}
	return all, nil
}

// CollectAllWorkspaces fetches all pages of workspaces for an organization.
func CollectAllWorkspaces(
	ctx context.Context,
	client *tfe.Client,
	org string,
	opts *tfe.WorkspaceListOptions,
) ([]*tfe.Workspace, error) {
	if opts == nil {
		opts = &tfe.WorkspaceListOptions{}
	}
	if opts.PageSize == 0 {
		opts.PageSize = DefaultPageSize
	}
	opts.PageNumber = 1

	var all []*tfe.Workspace
	for {
		list, err := client.Workspaces.List(ctx, org, opts)
		if err != nil {
			return nil, err
		}
		all = append(all, list.Items...)

		if len(list.Items) == 0 || list.Pagination == nil || list.Pagination.NextPage == 0 {
			break
		}
		opts.PageNumber = list.Pagination.NextPage
	}
	return all, nil
}

// CollectAllProjects fetches all pages of projects for an organization.
func CollectAllProjects(
	ctx context.Context,
	client *tfe.Client,
	org string,
	opts *tfe.ProjectListOptions,
) ([]*tfe.Project, error) {
	if opts == nil {
		opts = &tfe.ProjectListOptions{}
	}
	if opts.PageSize == 0 {
		opts.PageSize = DefaultPageSize
	}
	opts.PageNumber = 1

	var all []*tfe.Project
	for {
		list, err := client.Projects.List(ctx, org, opts)
		if err != nil {
			return nil, err
		}
		all = append(all, list.Items...)

		if len(list.Items) == 0 || list.Pagination == nil || list.Pagination.NextPage == 0 {
			break
		}
		opts.PageNumber = list.Pagination.NextPage
	}
	return all, nil
}

// CollectAllRuns fetches all pages of runs for a workspace.
func CollectAllRuns(
	ctx context.Context,
	client *tfe.Client,
	workspaceID string,
	opts *tfe.RunListOptions,
) ([]*tfe.Run, error) {
	return CollectRunsWithLimit(ctx, client, workspaceID, opts, 0)
}

// CollectRunsWithLimit fetches runs for a workspace up to a limit.
// If limit is 0 or negative, all runs are fetched.
func CollectRunsWithLimit(
	ctx context.Context,
	client *tfe.Client,
	workspaceID string,
	opts *tfe.RunListOptions,
	limit int,
) ([]*tfe.Run, error) {
	if opts == nil {
		opts = &tfe.RunListOptions{}
	}
	if opts.PageSize == 0 {
		opts.PageSize = DefaultPageSize
	}
	opts.PageNumber = 1

	var all []*tfe.Run
	for {
		list, err := client.Runs.List(ctx, workspaceID, opts)
		if err != nil {
			return nil, err
		}
		all = append(all, list.Items...)

		// If we've reached the limit, stop
		if limit > 0 && len(all) >= limit {
			all = all[:limit]
			break
		}

		if len(list.Items) == 0 || list.Pagination == nil || list.Pagination.NextPage == 0 {
			break
		}
		opts.PageNumber = list.Pagination.NextPage
	}
	return all, nil
}

// CollectAllConfigurationVersions fetches all pages of configuration versions for a workspace.
func CollectAllConfigurationVersions(
	ctx context.Context,
	client *tfe.Client,
	workspaceID string,
	opts *tfe.ConfigurationVersionListOptions,
) ([]*tfe.ConfigurationVersion, error) {
	if opts == nil {
		opts = &tfe.ConfigurationVersionListOptions{}
	}
	if opts.PageSize == 0 {
		opts.PageSize = DefaultPageSize
	}
	opts.PageNumber = 1

	var all []*tfe.ConfigurationVersion
	for {
		list, err := client.ConfigurationVersions.List(ctx, workspaceID, opts)
		if err != nil {
			return nil, err
		}
		all = append(all, list.Items...)

		if len(list.Items) == 0 || list.Pagination == nil || list.Pagination.NextPage == 0 {
			break
		}
		opts.PageNumber = list.Pagination.NextPage
	}
	return all, nil
}

// CollectAllVariables fetches all pages of variables for a workspace.
func CollectAllVariables(
	ctx context.Context,
	client *tfe.Client,
	workspaceID string,
	opts *tfe.VariableListOptions,
) ([]*tfe.Variable, error) {
	if opts == nil {
		opts = &tfe.VariableListOptions{}
	}
	if opts.PageSize == 0 {
		opts.PageSize = DefaultPageSize
	}
	opts.PageNumber = 1

	var all []*tfe.Variable
	for {
		list, err := client.Variables.List(ctx, workspaceID, opts)
		if err != nil {
			return nil, err
		}
		all = append(all, list.Items...)

		if len(list.Items) == 0 || list.Pagination == nil || list.Pagination.NextPage == 0 {
			break
		}
		opts.PageNumber = list.Pagination.NextPage
	}
	return all, nil
}

// CollectAllWorkspaceResources fetches all pages of workspace resources for a workspace.
func CollectAllWorkspaceResources(
	ctx context.Context,
	client *tfe.Client,
	workspaceID string,
	opts *tfe.WorkspaceResourceListOptions,
) ([]*tfe.WorkspaceResource, error) {
	if opts == nil {
		opts = &tfe.WorkspaceResourceListOptions{}
	}
	if opts.PageSize == 0 {
		opts.PageSize = DefaultPageSize
	}
	opts.PageNumber = 1

	var all []*tfe.WorkspaceResource
	for {
		list, err := client.WorkspaceResources.List(ctx, workspaceID, opts)
		if err != nil {
			return nil, err
		}
		all = append(all, list.Items...)

		if len(list.Items) == 0 || list.Pagination == nil || list.Pagination.NextPage == 0 {
			break
		}
		opts.PageNumber = list.Pagination.NextPage
	}
	return all, nil
}
