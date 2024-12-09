package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"

	"your-project/tenant"
)

func TestScriptRepoImpl_GetAll(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()

	// Sample test scripts
	testScripts := []*Script{
		{
			ID:     "script1",
			Name:   "Test Script 1",
			Tenant: "test-tenant",
		},
		{
			ID:     "script2", 
			Name:   "Test Script 2", 
			Tenant: "test-tenant",
		},
	}

	mt.Run("Successful GetAll with no filters", func(mt *mtest.T) {
		// Create a mock tenant
		testTenant := &tenant.Tenant{
			ID: "test-tenant",
		}

		// Setup the mock collection and cursor
		cursor, err := mtest.CreateCursorFromDocuments(testScripts...)
		assert.NoError(mt, err)

		// Configure the mock to return the cursor
		mt.AddMockResponses(mtest.CreateCursorResponse(1, "test.scripts", cursor))

		// Create repository with mock database
		repo := &ScriptRepoIml{db: mt.Client.Database("test")}

		// Call the method
		scripts, err := repo.GetAll(testTenant)

		// Assertions
		assert.NoError(mt, err)
		assert.Len(mt, scripts, 2)
		assert.Equal(mt, "script1", scripts[0].ID)
		assert.Equal(mt, "script2", scripts[1].ID)
	})

	mt.Run("Successful GetAll with filters", func(mt *mtest.T) {
		// Create a mock tenant
		testTenant := &tenant.Tenant{
			ID: "test-tenant",
		}

		// Prepare custom filter
		customFilter := map[string]interface{}{
			"name": "Test Script 1",
		}

		// Filtered test scripts
		filteredScripts := []*Script{
			{
				ID:     "script1",
				Name:   "Test Script 1",
				Tenant: "test-tenant",
			},
		}

		// Setup the mock collection and cursor
		cursor, err := mtest.CreateCursorFromDocuments(filteredScripts...)
		assert.NoError(mt, err)

		// Configure the mock to return the cursor
		mt.AddMockResponses(mtest.CreateCursorResponse(1, "test.scripts", cursor))

		// Create repository with mock database
		repo := &ScriptRepoIml{db: mt.Client.Database("test")}

		// Call the method with custom filter
		scripts, err := repo.GetAll(testTenant, customFilter)

		// Assertions
		assert.NoError(mt, err)
		assert.Len(mt, scripts, 1)
		assert.Equal(mt, "script1", scripts[0].ID)
	})

	mt.Run("Error in database query", func(mt *mtest.T) {
		// Create a mock tenant
		testTenant := &tenant.Tenant{
			ID: "test-tenant",
		}

		// Configure the mock to return an error
		mt.AddMockResponses(mtest.CreateWriteErrorsResponse(mtest.WriteError{
			Message: "database query error",
		}))

		// Create repository with mock database
		repo := &ScriptRepoIml{db: mt.Client.Database("test")}

		// Call the method
		scripts, err := repo.GetAll(testTenant)

		// Assertions
		assert.Error(mt, err)
		assert.Nil(mt, scripts)
	})

	mt.Run("Error in cursor processing", func(mt *mtest.T) {
		// Create a mock tenant
		testTenant := &tenant.Tenant{
			ID: "test-tenant",
		}

		// Setup a cursor that will cause an error when processing
		cursor, err := mtest.CreateCursorFromDocuments(testScripts...)
		assert.NoError(mt, err)

		// Configure the mock to return a valid cursor
		mt.AddMockResponses(mtest.CreateCursorResponse(1, "test.scripts", cursor))

		// Create repository with mock database
		repo := &ScriptRepoIml{db: mt.Client.Database("test")}

		// Simulate an error by closing the context
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Immediately cancel the context

		// Override the repository's context with the cancelled context
		repo.db = mt.Client.Database("test").WithContext(ctx)

		// Call the method
		scripts, err := repo.GetAll(testTenant)

		// Assertions
		assert.Error(mt, err)
		assert.Nil(mt, scripts)
	})
}

// Helper function to create a sample tenant for testing
func createTestTenant() *tenant.Tenant {
	return &tenant.Tenant{
		ID: "test-tenant",
	}
}