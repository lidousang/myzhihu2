package flags

import (
	"myzhihu2/repository/model"
	"myzhihu2/util/global"

	"fmt"
)

func DBMigrate() {
	err := global.DB.AutoMigrate(model.All()...)
	if err != nil {
		fmt.Printf("fail to migrate database: %s", err)
		return
	}
	fmt.Println("succeed to migrate database")
}
