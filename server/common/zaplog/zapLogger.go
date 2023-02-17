// ///////////////////////////////////////
// 2022 PJLab Storage all rights reserved
// Author: Yangyang Qian
// ///////////////////////////////////////
package zaplog

import (
	"fmt"
	"go.uber.org/zap"
	"os"
)

var ZapLogger *zap.Logger

func init() {
	var err error
	ZapLogger, err = zap.NewDevelopment()
	if err != nil {
		fmt.Printf("ZapLogger init failed. \n")
		os.Exit(1)
	}
	defer ZapLogger.Sync()
}
