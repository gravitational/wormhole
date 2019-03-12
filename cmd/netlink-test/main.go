/*
Copyright 2019 Gravitational, Inc.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/vishvananda/netlink"
)

func main() {
	routes, err := netlink.RouteList(nil, 0)
	if err != nil {
		fmt.Println("Error: ", spew.Sdump(err))
	}
	fmt.Println("Routes: ", spew.Sdump(routes))

	fmt.Println("wormhole-wg0")
	link, err := netlink.LinkByName("wormhole-wg0")
	fmt.Println("  err: ", spew.Sdump(err))
	fmt.Println("  link: ", spew.Sdump(link))

	fmt.Println("wormhole-br0")
	link, err = netlink.LinkByName("wormhole-br0")
	fmt.Println("  err: ", spew.Sdump(err))
	fmt.Println("  link: ", spew.Sdump(link))
}
