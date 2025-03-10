package permissions

import (
	"context"
	"net/http"
	"testing"

	"github.com/databrickslabs/terraform-provider-databricks/common"
	"github.com/databrickslabs/terraform-provider-databricks/identity"
	"github.com/databrickslabs/terraform-provider-databricks/jobs"

	"github.com/databrickslabs/terraform-provider-databricks/qa"
	"github.com/databrickslabs/terraform-provider-databricks/workspace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	TestingUser      = "ben"
	TestingAdminUser = "admin"
	me               = qa.HTTPFixture{
		ReuseRequest: true,
		Method:       "GET",
		Resource:     "/api/2.0/preview/scim/v2/Me",
		Response: identity.ScimUser{
			UserName: TestingAdminUser,
		},
	}
)

func TestAccessControlChangeString(t *testing.T) {
	assert.Equal(t, "me CAN_READ", AccessControlChange{
		UserName:        "me",
		PermissionLevel: "CAN_READ",
	}.String())
}

func TestAccessControlString(t *testing.T) {
	assert.Equal(t, "me[CAN_READ (from [parent]) CAN_MANAGE]", AccessControl{
		UserName: "me",
		AllPermissions: []Permission{
			{
				InheritedFromObject: []string{"parent"},
				PermissionLevel:     "CAN_READ",
			},
			{
				PermissionLevel: "CAN_MANAGE",
			},
		},
	}.String())
}

func TestResourcePermissionsRead(t *testing.T) {
	d, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			me,
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/permissions/clusters/abc",
				Response: ObjectACL{
					ObjectID:   "/clusters/abc",
					ObjectType: "cluster",
					AccessControlList: []AccessControl{
						{
							UserName: TestingUser,
							AllPermissions: []Permission{
								{
									PermissionLevel: "CAN_READ",
									Inherited:       false,
								},
							},
						},
						{
							UserName: TestingAdminUser,
							AllPermissions: []Permission{
								{
									PermissionLevel: "CAN_MANAGE",
									Inherited:       false,
								},
							},
						},
					},
				},
			},
		},
		Resource: ResourcePermissions(),
		Read:     true,
		New:      true,
		ID:       "/clusters/abc",
	}.Apply(t)
	assert.NoError(t, err, err)
	assert.Equal(t, "/clusters/abc", d.Id())
	ac := d.Get("access_control").(*schema.Set)
	require.Equal(t, 1, len(ac.List()))
	firstElem := ac.List()[0].(map[string]interface{})
	assert.Equal(t, TestingUser, firstElem["user_name"])
	assert.Equal(t, "CAN_READ", firstElem["permission_level"])
}

func TestResourcePermissionsRead_SQLA_Asset(t *testing.T) {
	d, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			me,
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/preview/sql/permissions/dashboards/abc",
				Response: ObjectACL{
					ObjectID:   "/sql/dashboards/abc",
					ObjectType: "dashboard",
					AccessControlList: []AccessControl{
						{
							UserName:        TestingUser,
							PermissionLevel: "CAN_READ",
						},
						{
							UserName:        TestingAdminUser,
							PermissionLevel: "CAN_MANAGE",
						},
					},
				},
			},
		},
		Resource: ResourcePermissions(),
		Read:     true,
		New:      true,
		ID:       "/sql/dashboards/abc",
	}.Apply(t)
	assert.NoError(t, err, err)
	assert.Equal(t, "/sql/dashboards/abc", d.Id())
	ac := d.Get("access_control").(*schema.Set)
	require.Equal(t, 1, len(ac.List()))
	firstElem := ac.List()[0].(map[string]interface{})
	assert.Equal(t, TestingUser, firstElem["user_name"])
	assert.Equal(t, "CAN_READ", firstElem["permission_level"])
}

func TestResourcePermissionsRead_NotFound(t *testing.T) {
	qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			me,
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/permissions/clusters/abc",
				Response: common.APIErrorBody{
					ErrorCode: "NOT_FOUND",
					Message:   "Cluster does not exist",
				},
				Status: 404,
			},
		},
		Resource: ResourcePermissions(),
		Read:     true,
		New:      true,
		Removed:  true,
		ID:       "/clusters/abc",
	}.ApplyNoError(t)
}

func TestResourcePermissionsRead_some_error(t *testing.T) {
	_, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			me,
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/permissions/clusters/abc",
				Response: common.APIErrorBody{
					ErrorCode: "INVALID_REQUEST",
					Message:   "Internal error happened",
				},
				Status: 400,
			},
		},
		Resource: ResourcePermissions(),
		Read:     true,
		ID:       "/clusters/abc",
	}.Apply(t)
	assert.Error(t, err)
}

func TestResourcePermissionsRead_ErrorOnScimMe(t *testing.T) {
	_, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/permissions/clusters/abc",
				Response: ObjectACL{
					ObjectID:   "/clusters/abc",
					ObjectType: "clusters",
					AccessControlList: []AccessControl{
						{
							UserName: TestingUser,
							AllPermissions: []Permission{
								{
									PermissionLevel: "CAN_READ",
									Inherited:       false,
								},
							},
						},
						{
							UserName: TestingAdminUser,
							AllPermissions: []Permission{
								{
									PermissionLevel: "CAN_MANAGE",
									Inherited:       false,
								},
							},
						},
					},
				},
			},
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/preview/scim/v2/Me",
				Response: common.APIErrorBody{
					ErrorCode: "INVALID_REQUEST",
					Message:   "Internal error happened",
				},
				Status: 400,
			},
		},
		Resource: ResourcePermissions(),
		Read:     true,
		ID:       "/clusters/abc",
	}.Apply(t)
	assert.Error(t, err)
}

func TestResourcePermissionsDelete(t *testing.T) {
	d, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			me,
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/permissions/clusters/abc",
				Response: ObjectACL{
					ObjectID:   "/clusters/abc",
					ObjectType: "clusters",
					AccessControlList: []AccessControl{
						{
							UserName: TestingUser,
							AllPermissions: []Permission{
								{
									PermissionLevel: "CAN_READ",
									Inherited:       false,
								},
							},
						},
						{
							UserName: TestingAdminUser,
							AllPermissions: []Permission{
								{
									PermissionLevel: "CAN_MANAGE",
									Inherited:       false,
								},
							},
						},
					},
				},
			},
			{
				Method:          http.MethodPut,
				Resource:        "/api/2.0/permissions/clusters/abc",
				ExpectedRequest: ObjectACL{},
			},
		},
		Resource: ResourcePermissions(),
		Delete:   true,
		ID:       "/clusters/abc",
	}.Apply(t)
	assert.NoError(t, err, err)
	assert.Equal(t, "/clusters/abc", d.Id())
}

func TestResourcePermissionsDelete_error(t *testing.T) {
	_, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			me,
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/permissions/clusters/abc",
				Response: ObjectACL{
					ObjectID:   "/clusters/abc",
					ObjectType: "clusters",
					AccessControlList: []AccessControl{
						{
							UserName: TestingUser,
							AllPermissions: []Permission{
								{
									PermissionLevel: "CAN_READ",
									Inherited:       false,
								},
							},
						},
						{
							UserName: TestingAdminUser,
							AllPermissions: []Permission{
								{
									PermissionLevel: "CAN_MANAGE",
									Inherited:       false,
								},
							},
						},
					},
				},
			},
			{
				Method:          http.MethodPut,
				Resource:        "/api/2.0/permissions/clusters/abc",
				ExpectedRequest: ObjectACL{},
				Response: common.APIErrorBody{
					ErrorCode: "INVALID_REQUEST",
					Message:   "Internal error happened",
				},
				Status: 400,
			},
		},
		Resource: ResourcePermissions(),
		Delete:   true,
		ID:       "/clusters/abc",
	}.Apply(t)
	assert.Error(t, err)
}

func TestResourcePermissionsCreate_invalid(t *testing.T) {
	_, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{me},
		Resource: ResourcePermissions(),
		Create:   true,
	}.Apply(t)
	qa.AssertErrorStartsWith(t, err, "At least one type of resource identifiers must be set")
}

func TestResourcePermissionsCreate_no_access_control(t *testing.T) {
	qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{},
		Resource: ResourcePermissions(),
		Create:   true,
		State: map[string]interface{}{
			"cluster_id": "abc",
		},
	}.ExpectError(t, "invalid config supplied. [access_control] Missing required argument")
}

func TestResourcePermissionsCreate_conflicting_fields(t *testing.T) {
	qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{},
		Resource: ResourcePermissions(),
		Create:   true,
		State: map[string]interface{}{
			"cluster_id":    "abc",
			"notebook_path": "/Init",
			"access_control": []interface{}{
				map[string]interface{}{
					"user_name":        TestingUser,
					"permission_level": "CAN_READ",
				},
			},
		},
	}.ExpectError(t, "invalid config supplied. [cluster_id] Conflicting configuration arguments. [notebook_path] Conflicting configuration arguments")
}

func TestResourcePermissionsCreate_AdminsThrowError(t *testing.T) {
	_, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{},
		Resource: ResourcePermissions(),
		Create:   true,
		HCL: `
		cluster_id = "abc"
		access_control {
			group_name = "admins"
			permission_level = "CAN_MANAGE"
		}
		`,
	}.Apply(t)
	assert.EqualError(t, err, "invalid config supplied. [access_control] "+
		"It is not possible to restrict any permissions from `admins`.")
}

func TestResourcePermissionsCreate(t *testing.T) {
	d, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			me,
			{
				Method:   http.MethodPut,
				Resource: "/api/2.0/permissions/clusters/abc",
				ExpectedRequest: AccessControlChangeList{
					AccessControlList: []AccessControlChange{
						{
							UserName:        TestingUser,
							PermissionLevel: "CAN_ATTACH_TO",
						},
					},
				},
			},
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/permissions/clusters/abc",
				Response: ObjectACL{
					ObjectID:   "/clusters/abc",
					ObjectType: "cluster",
					AccessControlList: []AccessControl{
						{
							UserName: TestingUser,
							AllPermissions: []Permission{
								{
									PermissionLevel: "CAN_ATTACH_TO",
									Inherited:       false,
								},
							},
						},
						{
							UserName: TestingAdminUser,
							AllPermissions: []Permission{
								{
									PermissionLevel: "CAN_MANAGE",
									Inherited:       false,
								},
							},
						},
					},
				},
			},
		},
		Resource: ResourcePermissions(),
		State: map[string]interface{}{
			"cluster_id": "abc",
			"access_control": []interface{}{
				map[string]interface{}{
					"user_name":        TestingUser,
					"permission_level": "CAN_ATTACH_TO",
				},
			},
		},
		Create: true,
	}.Apply(t)
	assert.NoError(t, err, err)
	ac := d.Get("access_control").(*schema.Set)
	require.Equal(t, 1, len(ac.List()))
	firstElem := ac.List()[0].(map[string]interface{})
	assert.Equal(t, TestingUser, firstElem["user_name"])
	assert.Equal(t, "CAN_ATTACH_TO", firstElem["permission_level"])
}

func TestResourcePermissionsCreate_SQLA_Asset(t *testing.T) {
	d, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			me,
			{
				Method:   http.MethodPost,
				Resource: "/api/2.0/preview/sql/permissions/dashboards/abc",
				ExpectedRequest: AccessControlChangeList{
					AccessControlList: []AccessControlChange{
						{
							UserName:        TestingUser,
							PermissionLevel: "CAN_RUN",
						},
						{
							UserName:        TestingAdminUser,
							PermissionLevel: "CAN_MANAGE",
						},
					},
				},
			},
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/preview/sql/permissions/dashboards/abc",
				Response: ObjectACL{
					ObjectID:   "/sql/dashboards/abc",
					ObjectType: "dashboard",
					AccessControlList: []AccessControl{
						{
							UserName:        TestingUser,
							PermissionLevel: "CAN_RUN",
						},
						{
							UserName:        TestingAdminUser,
							PermissionLevel: "CAN_MANAGE",
						},
					},
				},
			},
		},
		Resource: ResourcePermissions(),
		State: map[string]interface{}{
			"sql_dashboard_id": "abc",
			"access_control": []interface{}{
				map[string]interface{}{
					"user_name":        TestingUser,
					"permission_level": "CAN_RUN",
				},
			},
		},
		Create: true,
	}.Apply(t)
	assert.NoError(t, err, err)
	ac := d.Get("access_control").(*schema.Set)
	require.Equal(t, 1, len(ac.List()))
	firstElem := ac.List()[0].(map[string]interface{})
	assert.Equal(t, TestingUser, firstElem["user_name"])
	assert.Equal(t, "CAN_RUN", firstElem["permission_level"])
}

func TestResourcePermissionsCreate_SQLA_Endpoint(t *testing.T) {
	d, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			me,
			{
				Method:   http.MethodPatch,
				Resource: "/api/2.0/permissions/sql/endpoints/abc",
				ExpectedRequest: AccessControlChangeList{
					AccessControlList: []AccessControlChange{
						{
							UserName:        TestingUser,
							PermissionLevel: "CAN_USE",
						},
						{
							UserName:        TestingAdminUser,
							PermissionLevel: "CAN_MANAGE",
						},
					},
				},
			},
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/permissions/sql/endpoints/abc",
				Response: ObjectACL{
					ObjectID:   "/sql/dashboards/abc",
					ObjectType: "dashboard",
					AccessControlList: []AccessControl{
						{
							UserName:        TestingUser,
							PermissionLevel: "CAN_USE",
						},
						{
							UserName:        TestingAdminUser,
							PermissionLevel: "CAN_MANAGE",
						},
					},
				},
			},
		},
		Resource: ResourcePermissions(),
		State: map[string]interface{}{
			"sql_endpoint_id": "abc",
			"access_control": []interface{}{
				map[string]interface{}{
					"user_name":        TestingUser,
					"permission_level": "CAN_USE",
				},
			},
		},
		Create: true,
	}.Apply(t)
	assert.NoError(t, err, err)
	ac := d.Get("access_control").(*schema.Set)
	require.Equal(t, 1, len(ac.List()))
	firstElem := ac.List()[0].(map[string]interface{})
	assert.Equal(t, TestingUser, firstElem["user_name"])
	assert.Equal(t, "CAN_USE", firstElem["permission_level"])
}

func TestResourcePermissionsCreate_NotebookPath_NotExists(t *testing.T) {
	_, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			me,
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/workspace/get-status?path=%2FDevelopment%2FInit",
				Response: common.APIErrorBody{
					ErrorCode: "INVALID_REQUEST",
					Message:   "Internal error happened",
				},
				Status: 400,
			},
		},
		Resource: ResourcePermissions(),
		State: map[string]interface{}{
			"notebook_path": "/Development/Init",
			"access_control": []interface{}{
				map[string]interface{}{
					"user_name":        TestingUser,
					"permission_level": "CAN_USE",
				},
			},
		},
		Create: true,
	}.Apply(t)

	assert.Error(t, err)
}

func TestResourcePermissionsCreate_NotebookPath(t *testing.T) {
	d, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			me,
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/workspace/get-status?path=%2FDevelopment%2FInit",
				Response: workspace.ObjectStatus{
					ObjectID:   988765,
					ObjectType: "NOTEBOOK",
				},
			},
			{
				Method:   http.MethodPut,
				Resource: "/api/2.0/permissions/notebooks/988765",
				ExpectedRequest: AccessControlChangeList{
					AccessControlList: []AccessControlChange{
						{
							UserName:        TestingUser,
							PermissionLevel: "CAN_READ",
						},
					},
				},
			},
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/permissions/notebooks/988765",
				Response: ObjectACL{
					ObjectID:   "/notebooks/988765",
					ObjectType: "notebook",
					AccessControlList: []AccessControl{
						{
							UserName: TestingUser,
							AllPermissions: []Permission{
								{
									PermissionLevel: "CAN_READ",
									Inherited:       false,
								},
							},
						},
						{
							UserName: TestingAdminUser,
							AllPermissions: []Permission{
								{
									PermissionLevel: "CAN_MANAGE",
									Inherited:       false,
								},
							},
						},
					},
				},
			},
		},
		Resource: ResourcePermissions(),
		State: map[string]interface{}{
			"notebook_path": "/Development/Init",
			"access_control": []interface{}{
				map[string]interface{}{
					"user_name":        TestingUser,
					"permission_level": "CAN_READ",
				},
			},
		},
		Create: true,
	}.Apply(t)

	assert.NoError(t, err, err)
	ac := d.Get("access_control").(*schema.Set)
	require.Equal(t, 1, len(ac.List()))
	firstElem := ac.List()[0].(map[string]interface{})
	assert.Equal(t, TestingUser, firstElem["user_name"])
	assert.Equal(t, "CAN_READ", firstElem["permission_level"])
}

func TestResourcePermissionsCreate_error(t *testing.T) {
	_, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			me,
			{
				Method:   http.MethodPut,
				Resource: "/api/2.0/permissions/clusters/abc",
				Response: common.APIErrorBody{
					ErrorCode: "INVALID_REQUEST",
					Message:   "Internal error happened",
				},
				Status: 400,
			},
		},
		Resource: ResourcePermissions(),
		State: map[string]interface{}{
			"cluster_id": "abc",
			"access_control": []interface{}{
				map[string]interface{}{
					"user_name":        TestingUser,
					"permission_level": "CAN_USE",
				},
			},
		},
		Create: true,
	}.Apply(t)
	if assert.Error(t, err) {
		if e, ok := err.(common.APIError); ok {
			assert.Equal(t, "INVALID_REQUEST", e.ErrorCode)
		}
	}
}

func TestResourcePermissionsUpdate(t *testing.T) {
	d, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			me,
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/permissions/jobs/9",
				Response: ObjectACL{
					ObjectID:   "/jobs/9",
					ObjectType: "job",
					AccessControlList: []AccessControl{
						{
							UserName: TestingUser,
							AllPermissions: []Permission{
								{
									PermissionLevel: "CAN_VIEW",
									Inherited:       false,
								},
							},
						},
						{
							UserName: TestingAdminUser,
							AllPermissions: []Permission{
								{
									PermissionLevel: "CAN_MANAGE",
									Inherited:       false,
								},
							},
						},
					},
				},
			},
			{
				Method:   http.MethodPut,
				Resource: "/api/2.0/permissions/jobs/9",
				ExpectedRequest: AccessControlChangeList{
					AccessControlList: []AccessControlChange{
						{
							UserName:        TestingUser,
							PermissionLevel: "CAN_VIEW",
						},
						{
							UserName:        TestingAdminUser,
							PermissionLevel: "IS_OWNER",
						},
					},
				},
			},
		},
		InstanceState: map[string]string{
			"job_id": "9",
		},
		HCL: `
		job_id = 9

		access_control {
			user_name = "ben"
			permission_level = "CAN_VIEW"
		}
		`,
		Resource: ResourcePermissions(),
		Update:   true,
		ID:       "/jobs/9",
	}.Apply(t)
	assert.NoError(t, err, err)
	assert.Equal(t, "/jobs/9", d.Id())
	ac := d.Get("access_control").(*schema.Set)
	require.Equal(t, 1, len(ac.List()))
	firstElem := ac.List()[0].(map[string]interface{})
	assert.Equal(t, TestingUser, firstElem["user_name"])
	assert.Equal(t, "CAN_VIEW", firstElem["permission_level"])
}

func TestResourcePermissionsUpdateTokensAlwaysThereForAdmins(t *testing.T) {
	qa.HTTPFixturesApply(t, []qa.HTTPFixture{
		{
			Method:   "PUT",
			Resource: "/api/2.0/permissions/authorization/tokens",
			ExpectedRequest: AccessControlChangeList{
				AccessControlList: []AccessControlChange{
					{
						UserName:        "me",
						PermissionLevel: "CAN_MANAGE",
					},
					{
						GroupName:       "admins",
						PermissionLevel: "CAN_MANAGE",
					},
				},
			},
		},
	}, func(ctx context.Context, client *common.DatabricksClient) {
		p := NewPermissionsAPI(ctx, client)
		err := p.Update("/authorization/tokens", AccessControlChangeList{
			AccessControlList: []AccessControlChange{
				{
					UserName:        "me",
					PermissionLevel: "CAN_MANAGE",
				},
			},
		})
		assert.NoError(t, err)
	})
}

func TestShouldKeepAdminsOnAnythingExceptPasswordsAndAssignsOwnerForJob(t *testing.T) {
	qa.HTTPFixturesApply(t, []qa.HTTPFixture{
		{
			Method:   "GET",
			Resource: "/api/2.0/permissions/jobs/123",
			Response: ObjectACL{
				ObjectID:   "/jobs/123",
				ObjectType: "job",
				AccessControlList: []AccessControl{
					{
						GroupName: "admins",
						AllPermissions: []Permission{
							{
								PermissionLevel: "CAN_DO_EVERYTHING",
								Inherited:       true,
							},
							{
								PermissionLevel: "CAN_MANAGE",
								Inherited:       false,
							},
						},
					},
				},
			},
		},
		{
			Method:   "GET",
			Resource: "/api/2.0/jobs/get?job_id=123",
			Response: jobs.Job{
				CreatorUserName: "creator@example.com",
			},
		},
		{
			Method:   "PUT",
			Resource: "/api/2.0/permissions/jobs/123",
			ExpectedRequest: ObjectACL{
				AccessControlList: []AccessControl{
					{
						GroupName:       "admins",
						PermissionLevel: "CAN_MANAGE",
					},
					{
						UserName:        "creator@example.com",
						PermissionLevel: "IS_OWNER",
					},
				},
			},
		},
	}, func(ctx context.Context, client *common.DatabricksClient) {
		p := NewPermissionsAPI(ctx, client)
		err := p.Delete("/jobs/123")
		assert.NoError(t, err)
	})
}

func TestCustomizeDiffNoHostYet(t *testing.T) {
	assert.Nil(t, ResourcePermissions().CustomizeDiff(context.TODO(), nil, &common.DatabricksClient{}))
}

func TestPathPermissionsResourceIDFields(t *testing.T) {
	var m permissionsIDFieldMapping
	for _, x := range permissionsResourceIDFields() {
		if x.field == "notebook_path" {
			m = x
		}
	}
	_, err := m.idRetriever(context.Background(), &common.DatabricksClient{
		Host:  "localhost",
		Token: "x",
	}, "x")
	assert.EqualError(t, err, "Cannot load path x: DatabricksClient is not configured")
}

func TestObjectACLToPermissionsEntityCornerCases(t *testing.T) {
	_, err := (&ObjectACL{
		ObjectType: "bananas",
		AccessControlList: []AccessControl{
			{
				GroupName: "admins",
			},
		},
	}).ToPermissionsEntity(ResourcePermissions().TestResourceData(), "me")
	assert.EqualError(t, err, "unknown object type bananas")
}

func TestAccessControlToAccessControlChange(t *testing.T) {
	_, res := AccessControl{}.toAccessControlChange()
	assert.False(t, res)
}

func TestCornerCases(t *testing.T) {
	qa.ResourceCornerCases(t, ResourcePermissions(), qa.CornerCaseSkipCRUD("create"))
}

func TestDeleteMissing(t *testing.T) {
	qa.HTTPFixturesApply(t, []qa.HTTPFixture{
		{
			MatchAny: true,
			Status:   404,
			Response: common.NotFound("missing"),
		},
	}, func(ctx context.Context, client *common.DatabricksClient) {
		p := ResourcePermissions()
		d := p.TestResourceData()
		d.SetId("x")
		diags := p.DeleteContext(ctx, d, client)
		assert.Nil(t, diags)
	})
}

func TestResourcePermissionsCreate_RepoPath(t *testing.T) {
	d, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			me,
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/workspace/get-status?path=%2FRepos%2FDevelopment%2FInit",
				Response: workspace.ObjectStatus{
					ObjectID:   988765,
					ObjectType: "repo",
				},
			},
			{
				Method:   http.MethodPut,
				Resource: "/api/2.0/permissions/repos/988765",
				ExpectedRequest: AccessControlChangeList{
					AccessControlList: []AccessControlChange{
						{
							UserName:        TestingUser,
							PermissionLevel: "CAN_READ",
						},
					},
				},
			},
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/permissions/repos/988765",
				Response: ObjectACL{
					ObjectID:   "/repos/988765",
					ObjectType: "repo",
					AccessControlList: []AccessControl{
						{
							UserName: TestingUser,
							AllPermissions: []Permission{
								{
									PermissionLevel: "CAN_READ",
									Inherited:       false,
								},
							},
						},
						{
							UserName: TestingAdminUser,
							AllPermissions: []Permission{
								{
									PermissionLevel: "CAN_RUN",
									Inherited:       false,
								},
							},
						},
						{
							UserName: TestingAdminUser,
							AllPermissions: []Permission{
								{
									PermissionLevel: "CAN_MANAGE",
									Inherited:       false,
								},
							},
						},
					},
				},
			},
		},
		Resource: ResourcePermissions(),
		State: map[string]interface{}{
			"repo_path": "/Repos/Development/Init",
			"access_control": []interface{}{
				map[string]interface{}{
					"user_name":        TestingUser,
					"permission_level": "CAN_READ",
				},
			},
		},
		Create: true,
	}.Apply(t)

	assert.NoError(t, err, err)
	ac := d.Get("access_control").(*schema.Set)
	require.Equal(t, 1, len(ac.List()))
	firstElem := ac.List()[0].(map[string]interface{})
	assert.Equal(t, TestingUser, firstElem["user_name"])
	assert.Equal(t, "CAN_READ", firstElem["permission_level"])
}
