// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Script.sol";
import "../src/AEPBounty.sol";
import "../src/AEPReputation.sol";

contract DeployAll is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        vm.startBroadcast(deployerPrivateKey);

        // Step 1: Deploy AEPBounty
        AEPBounty bounty = new AEPBounty();
        console.log("AEPBounty deployed at:", address(bounty));

        // Step 2: Deploy AEPReputation (with Bounty address)
        AEPReputation reputation = new AEPReputation(address(bounty));
        console.log("AEPReputation deployed at:", address(reputation));

        // Step 3: Link reputation to bounty
        bounty.setReputationContract(address(reputation));
        console.log("Reputation contract linked to bounty");

        vm.stopBroadcast();
    }
}
