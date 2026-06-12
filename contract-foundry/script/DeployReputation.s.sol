// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Script.sol";
import "../src/AEPReputation.sol";

contract DeployReputation is Script {
    function run() external {
        address bountyAddr = vm.envAddress("REPUTATION_BOUNTY_ADDR");

        vm.startBroadcast();

        AEPReputation reputation = new AEPReputation(bountyAddr);
        console.log("AEPReputation deployed at:", address(reputation));

        vm.stopBroadcast();
    }
}
