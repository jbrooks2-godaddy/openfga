package validatemodels

import (
	"context"
	"fmt"
	"testing"

	parser "github.com/craigpastro/openfga-dsl-parser/v2"
	"github.com/oklog/ulid/v2"
	"github.com/openfga/openfga/cmd"
	"github.com/openfga/openfga/cmd/util"
	"github.com/openfga/openfga/pkg/typesystem"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	openfgapb "go.buf.build/openfga/go/openfga/api/openfga/v1"
)

func TestValidationResult(t *testing.T) {
	engines := []string{"postgres", "mysql"}

	totalStores := 200
	totalModelsForOneStore := 200

	for _, engine := range engines {
		t.Run(engine, func(t *testing.T) {
			_, ds, _, err := util.MustBootstrapDatastore(t, engine)
			require.NoError(t, err)

			ctx := context.Background()

			// write a bunch of stores (to trigger pagination)
			var storeID string
			for i := 0; i < totalStores; i++ {
				storeID = ulid.Make().String()
				_, err := ds.CreateStore(ctx, &openfgapb.Store{
					Id:   storeID,
					Name: fmt.Sprintf("store-%d", i),
				})
				require.NoError(t, err)
				t.Logf("added store %s\n", storeID)
			}

			// for the last store, write a bunch of models (to trigger pagination)
			for j := 0; j < totalModelsForOneStore; j++ {
				modelID := ulid.Make().String()
				err = ds.WriteAuthorizationModel(ctx, storeID, &openfgapb.AuthorizationModel{
					Id:            modelID,
					SchemaVersion: typesystem.SchemaVersion1_1,
					// invalid
					TypeDefinitions: parser.MustParse(`
									type document
									  relations
										define viewer:[user] as self
									`),
				})
				require.NoError(t, err)
				t.Logf("added model %s for store %s\n", modelID, storeID)
			}

			t.Run("validate returns success", func(t *testing.T) {
				validationResults, err := ValidateAllAuthorizationModels(ctx, ds)
				require.NoError(t, err)
				require.Equal(t, totalModelsForOneStore, len(validationResults))
				require.Contains(t, "the relation type 'user' on 'viewer' in object type 'document' is not valid", validationResults[0].Error)
				require.Equal(t, true, validationResults[0].IsLatestModel)

				require.Contains(t, "the relation type 'user' on 'viewer' in object type 'document' is not valid", validationResults[1].Error)
				require.Equal(t, false, validationResults[1].IsLatestModel)
			})
		})
	}
}

func TestValidateModelsCommandWhenInvalidEngine(t *testing.T) {
	for _, tc := range []struct {
		engine        string
		errorExpected string
	}{
		{
			engine:        "memory",
			errorExpected: "storage engine 'memory' is unsupported",
		},
		{
			engine:        "",
			errorExpected: "missing datastore engine type",
		},
	} {
		t.Run(tc.engine, func(t *testing.T) {
			validateModelsCommand := NewValidateCommand()
			validateModelsCommand.SetArgs([]string{"--datastore-engine", tc.engine, "--datastore-uri", ""})
			err := validateModelsCommand.Execute()
			require.ErrorContains(t, err, tc.errorExpected)
		})
	}
}

func TestValidateModelsCommandNoConfigDefaultValues(t *testing.T) {
	util.PrepareTempConfigDir(t)
	validateCommand := NewValidateCommand()
	validateCommand.RunE = func(cmd *cobra.Command, _ []string) error {
		require.Equal(t, "", viper.GetString(datastoreEngineFlag))
		require.Equal(t, "", viper.GetString(datastoreURIFlag))
		return nil
	}

	cmd := cmd.NewRootCommand()
	cmd.AddCommand(validateCommand)
	cmd.SetArgs([]string{"validate-models"})
	require.Nil(t, cmd.Execute())
}

func TestValidateModelsCommandConfigFileValuesAreParsed(t *testing.T) {
	config := `datastore:
    engine: someEngine
    uri: postgres://postgres:password@127.0.0.1:5432/postgres
`
	util.PrepareTempConfigFile(t, config)

	validateCmd := NewValidateCommand()
	validateCmd.RunE = func(cmd *cobra.Command, _ []string) error {
		require.Equal(t, "someEngine", viper.GetString(datastoreEngineFlag))
		require.Equal(t, "postgres://postgres:password@127.0.0.1:5432/postgres", viper.GetString(datastoreURIFlag))
		return nil
	}

	cmd := cmd.NewRootCommand()
	cmd.AddCommand(validateCmd)
	cmd.SetArgs([]string{"validate-models"})
	require.Nil(t, cmd.Execute())
}

func TestValidateModelsCommandConfigIsMerged(t *testing.T) {
	config := `datastore:
    engine: anotherEngine
`
	util.PrepareTempConfigFile(t, config)

	t.Setenv("OPENFGA_DATASTORE_URI", "postgres://postgres:PASS2@127.0.0.1:5432/postgres")

	validateCmd := NewValidateCommand()
	validateCmd.RunE = func(cmd *cobra.Command, _ []string) error {
		require.Equal(t, "anotherEngine", viper.GetString(datastoreEngineFlag))
		require.Equal(t, "postgres://postgres:PASS2@127.0.0.1:5432/postgres", viper.GetString(datastoreURIFlag))
		return nil
	}

	cmd := cmd.NewRootCommand()
	cmd.AddCommand(validateCmd)
	cmd.SetArgs([]string{"validate-models"})
	require.Nil(t, cmd.Execute())
}
