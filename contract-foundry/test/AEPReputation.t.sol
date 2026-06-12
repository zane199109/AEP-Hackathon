// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Test.sol";
import "../src/AEPReputation.sol";

contract AEPReputationTest is Test {
    AEPReputation public rep;
    address public bountyContract = address(0x100);
    address public agent = address(0x200);
    address public caller2 = address(0x300);
    address public owner;

    bytes32 constant SAMPLE_HASH = keccak256("test_params");

    event ScoreUpdated(
        address indexed agent,
        uint256 newScore,
        int256 delta,
        uint256 indexed taskId,
        string reason,
        address indexed source,
        bytes32 paramHash
    );

    event AuthorizedCallerAdded(address indexed caller);
    event AuthorizedCallerRemoved(address indexed caller);

    function setUp() public {
        owner = address(this);
        rep = new AEPReputation(bountyContract);
    }

    // ============ 部署 & 权限 ============

    function test_Constructor() public {
        assertEq(rep.owner(), owner);
        assertTrue(rep.authorizedCallers(bountyContract));
        assertEq(rep.getScore(agent), 0);
    }

    function test_AddAuthorizedCaller() public {
        vm.expectEmit(true, false, false, true);
        emit AuthorizedCallerAdded(caller2);
        rep.addAuthorizedCaller(caller2);
        assertTrue(rep.authorizedCallers(caller2));
    }

    function test_RemoveAuthorizedCaller() public {
        rep.addAuthorizedCaller(caller2);
        assertTrue(rep.authorizedCallers(caller2));

        vm.expectEmit(true, false, false, true);
        emit AuthorizedCallerRemoved(caller2);
        rep.removeAuthorizedCaller(caller2);
        assertFalse(rep.authorizedCallers(caller2));
    }

    function test_RevertWhen_NonOwnerAddsCaller() public {
        vm.prank(caller2);
        vm.expectRevert("AEPReputation: not owner");
        rep.addAuthorizedCaller(caller2);
    }

    // ============ 核心：分数更新 ============

    function test_UpdateScore() public {
        vm.prank(bountyContract);
        vm.expectEmit(true, true, true, true);
        emit ScoreUpdated(agent, 85, 85, 1, "delivery_passed", bountyContract, SAMPLE_HASH);
        rep.updateScore(agent, 1, 85, 85, "delivery_passed", SAMPLE_HASH);

        assertEq(rep.getScore(agent), 85);
        assertTrue(rep.isTaskProcessed(agent, 1));
    }

    function test_UpdateScoreNegative() public {
        vm.prank(bountyContract);
        // First set score to 75, then slash
        rep.updateScore(agent, 2, 75, 75, "set_score", SAMPLE_HASH);
        rep.updateScore(agent, 3, 60, -15, "delivery_slashed", SAMPLE_HASH);

        assertEq(rep.getScore(agent), 60);
    }

    function test_RevertWhen_TaskAlreadyProcessed() public {
        vm.startPrank(bountyContract);
        rep.updateScore(agent, 1, 85, 85, "passed", SAMPLE_HASH);
        vm.expectRevert("AEPReputation: task already processed");
        rep.updateScore(agent, 1, 90, 5, "double", SAMPLE_HASH);
        vm.stopPrank();
    }

    function test_RevertWhen_UnauthorizedCaller() public {
        vm.prank(caller2);
        vm.expectRevert("AEPReputation: not authorized");
        rep.updateScore(agent, 1, 50, 0, "unauthorized", SAMPLE_HASH);
    }

    function test_OwnerIsAuthorized() public {
        rep.updateScore(agent, 1, 75, 75, "owner_update", SAMPLE_HASH);
        assertEq(rep.getScore(agent), 75);
    }

    // ============ 历史记录 ============

    function test_History() public {
        vm.startPrank(bountyContract);

        rep.updateScore(agent, 1, 70, 70, "task_1", SAMPLE_HASH);
        rep.updateScore(agent, 2, 80, 10, "task_2", SAMPLE_HASH);

        vm.stopPrank();

        assertEq(rep.getHistoryCount(agent), 2);

        AEPReputation.ReputationEvent[] memory events = rep.getHistory(agent, 0, 10);
        assertEq(events.length, 2);
        assertEq(events[0].taskId, 1);
        assertEq(events[0].delta, 70);
        assertEq(events[0].reason, "task_1");
        assertEq(events[0].source, bountyContract);
        assertEq(events[0].paramHash, SAMPLE_HASH);

        assertEq(events[1].taskId, 2);
        assertEq(events[1].delta, 10);

        // Pagination
        AEPReputation.ReputationEvent[] memory page1 = rep.getHistory(agent, 0, 1);
        assertEq(page1.length, 1);
        assertEq(page1[0].taskId, 1);

        AEPReputation.ReputationEvent[] memory page2 = rep.getHistory(agent, 1, 1);
        assertEq(page2.length, 1);
        assertEq(page2[0].taskId, 2);

        // Empty page
        AEPReputation.ReputationEvent[] memory empty = rep.getHistory(agent, 10, 5);
        assertEq(empty.length, 0);
    }

    // ============ 批量查询 ============

    function test_GetScores() public {
        vm.startPrank(bountyContract);
        rep.updateScore(agent, 1, 70, 70, "score_1", SAMPLE_HASH);

        address agent2 = address(0x201);
        rep.updateScore(agent2, 1, 90, 90, "score_2", SAMPLE_HASH);
        vm.stopPrank();

        address[] memory agents = new address[](2);
        agents[0] = agent;
        agents[1] = agent2;

        uint256[] memory scores = rep.getScores(agents);
        assertEq(scores.length, 2);
        assertEq(scores[0], 70);
        assertEq(scores[1], 90);
    }

    // ============ 所有权转移 ============

    function test_TransferOwnership() public {
        rep.transferOwnership(caller2);
        assertEq(rep.owner(), caller2);

        vm.prank(caller2);
        rep.addAuthorizedCaller(address(0x400));
        assertTrue(rep.authorizedCallers(address(0x400)));
    }

    function test_RevertWhen_NonOwnerTransfersOwnership() public {
        vm.prank(caller2);
        vm.expectRevert("AEPReputation: not owner");
        rep.transferOwnership(caller2);
    }

    function test_RevertWhen_TransferToZero() public {
        vm.expectRevert("AEPReputation: invalid address");
        rep.transferOwnership(address(0));
    }

    // ============ 边界情况 ============

    function test_ScoreCanBeZero() public {
        vm.startPrank(bountyContract);
        rep.updateScore(agent, 1, 50, 50, "set_score", SAMPLE_HASH);
        rep.updateScore(agent, 2, 0, -50, "zero_score", SAMPLE_HASH);
        vm.stopPrank();
        assertEq(rep.getScore(agent), 0);
    }

    function test_ScoreCanBeOneHundred() public {
        vm.prank(bountyContract);
        rep.updateScore(agent, 1, 100, 100, "max_score", SAMPLE_HASH);
        assertEq(rep.getScore(agent), 100);
    }

    function test_RevertWhen_ScoreOverOneHundred() public {
        vm.prank(bountyContract);
        vm.expectRevert("AEPReputation: score out of range");
        rep.updateScore(agent, 1, 101, 51, "over_max", SAMPLE_HASH);
    }

    function test_RevertWhen_DeltaMismatch() public {
        vm.prank(bountyContract);
        vm.expectRevert("AEPReputation: delta mismatch");
        rep.updateScore(agent, 1, 80, 100, "wrong_delta", SAMPLE_HASH);
    }

    function test_RevertWhen_RemoveUnauthorizedCaller() public {
        vm.expectRevert("AEPReputation: caller not authorized");
        rep.removeAuthorizedCaller(caller2);
    }

    function test_MultipleAgentsIndependent() public {
        address agent2 = address(0x201);
        vm.startPrank(bountyContract);

        rep.updateScore(agent, 1, 70, 70, "agent1", SAMPLE_HASH);
        rep.updateScore(agent2, 1, 90, 90, "agent2", SAMPLE_HASH);

        vm.stopPrank();

        assertEq(rep.getScore(agent), 70);
        assertEq(rep.getScore(agent2), 90);
        assertTrue(rep.isTaskProcessed(agent, 1));
        assertTrue(rep.isTaskProcessed(agent2, 1));
    }
}
