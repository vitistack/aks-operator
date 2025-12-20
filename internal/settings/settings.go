package settings

import (
	"github.com/spf13/viper"
	"github.com/vitistack/aks-operator/internal/consts"
	"github.com/vitistack/common/pkg/settings/dotenv"
)

func Init() {
	viper.SetDefault(consts.DEVELOPMENT, false)
	viper.SetDefault(consts.LOG_JSON, true)
	viper.SetDefault(consts.LOG_LEVEL, "info")
	viper.SetDefault(consts.VITISTACK_NAME, "vitistack")
	viper.SetDefault(consts.NAME_KUBERNETES_PROVIDER, "aks-provider")
	viper.SetDefault(consts.TENANT_CONFIGMAP_NAME, "aks-tenant-config")
	viper.SetDefault(consts.TENANT_CONFIGMAP_NAMESPACE, "default")
	viper.SetDefault(consts.TENANT_CONFIGMAP_DATA_KEY, "config.yaml")
	viper.SetDefault(consts.DEFAULT_KUBERNETES_VERSION, "1.34.1")
	viper.SetDefault(consts.AZURE_VALIDATE_CONNECTIVITY, true)

	dotenv.LoadDotEnv()
	viper.AutomaticEnv()
}
