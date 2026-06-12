// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/**
 * @title AEPBounty
 * @notice 密码学资金锁定调度 — 技术协议演示，仅使用测试网资产
 * @dev Base Sepolia 测试网部署
 *
 * 状态机: Open → Assigned → Submitted → Verified | Slashed
 *                \                    → Refunded (超时退款)
 */

// ============ ERC-8004 极简身份接口 ============

interface IAgentReputation {
    function updateScore(address agent, bool success) external;
    function getScore(address agent) external view returns (uint256);
}

contract AEPBounty {
    // ============ 类型 ============

    enum BountyStatus { Open, Assigned, Submitted, Verified, Slashed, Refunded }

    struct Bounty {
        address buyer;
        address seller;
        uint256 amount;
        uint256 deadline;
        string resultHash;      // IPFS hash of delivered work
        BountyStatus status;
    }

    // ============ 状态 ============

    uint256 public jobCount;
    mapping(uint256 => Bounty) public bounties;

    // ERC-8004 声誉层
    IAgentReputation public reputation;
    address public owner;

    // ============ 事件 ============

    event BountyPosted(uint256 indexed jobId, address indexed buyer, uint256 amount, uint256 deadline);
    event BountyClaimed(uint256 indexed jobId, address indexed seller);
    event ResultSubmitted(uint256 indexed jobId, string resultHash);
    event JobVerified(uint256 indexed jobId);
    event JobSlashed(uint256 indexed jobId);
    event Refunded(uint256 indexed jobId);

    // ============ 修饰器 ============

    modifier onlyBuyer(uint256 jobId) {
        require(msg.sender == bounties[jobId].buyer, "only buyer");
        _;
    }

    modifier onlySeller(uint256 jobId) {
        require(msg.sender == bounties[jobId].seller, "only seller");
        _;
    }

    modifier inStatus(uint256 jobId, BountyStatus expected) {
        require(bounties[jobId].status == expected, "invalid status");
        _;
    }

    modifier onlyOwner() {
        require(msg.sender == owner, "only owner");
        _;
    }

    // ============ 构造函数 ============

    constructor() {
        owner = msg.sender;
    }

    // ============ 核心函数 ============

    /// @notice 买家发榜并锁定资金
    function postBounty(uint256 deadline) external payable {
        require(msg.value > 0, "amount must be > 0");
        require(deadline > block.timestamp, "deadline must be in future");

        jobCount++;
        bounties[jobCount] = Bounty({
            buyer: msg.sender,
            seller: address(0),
            amount: msg.value,
            deadline: deadline,
            resultHash: "",
            status: BountyStatus.Open
        });

        emit BountyPosted(jobCount, msg.sender, msg.value, deadline);
    }

    /// @notice 卖家抢单
    function claimBounty(uint256 jobId) external
        inStatus(jobId, BountyStatus.Open)
    {
        require(block.timestamp <= bounties[jobId].deadline, "deadline passed");

        bounties[jobId].seller = msg.sender;
        bounties[jobId].status = BountyStatus.Assigned;

        emit BountyClaimed(jobId, msg.sender);
    }

    /// @notice 卖家提交交付结果 (IPFS hash)
    function submitResult(uint256 jobId, string calldata resultHash) external
        onlySeller(jobId)
        inStatus(jobId, BountyStatus.Assigned)
    {
        require(bytes(resultHash).length > 0, "resultHash cannot be empty");

        bounties[jobId].resultHash = resultHash;
        bounties[jobId].status = BountyStatus.Submitted;

        emit ResultSubmitted(jobId, resultHash);
    }

    /// @notice 验证工作结果（双轨裁决后调用）
    /// @param passed true = 通过, false = 判定欺诈/不合格→Slashed
    function verifyJobResult(uint256 jobId, bool passed) external
        onlyBuyer(jobId)
        inStatus(jobId, BountyStatus.Submitted)
    {
        if (passed) {
            bounties[jobId].status = BountyStatus.Verified;
            emit JobVerified(jobId);
        } else {
            bounties[jobId].status = BountyStatus.Slashed;
            emit JobSlashed(jobId);
        }

        // 一步到位更新链上声誉
        if (address(reputation) != address(0)) {
            reputation.updateScore(bounties[jobId].seller, passed);
        }
    }

    /// @notice 超时退款 — deadline 过后买家可回收资金
    function refundAfterTimeout(uint256 jobId) external
        onlyBuyer(jobId)
        inStatus(jobId, BountyStatus.Open)
    {
        require(block.timestamp > bounties[jobId].deadline, "deadline not passed");

        uint256 amount = bounties[jobId].amount;
        bounties[jobId].status = BountyStatus.Refunded;
        bounties[jobId].amount = 0;

        (bool sent, ) = payable(msg.sender).call{value: amount}("");
        require(sent, "refund transfer failed");

        emit Refunded(jobId);
    }

    // ============ 查询 ============

    function getBounty(uint256 jobId) external view returns (
        address buyer,
        address seller,
        uint256 amount,
        uint256 deadline,
        string memory resultHash,
        BountyStatus status
    ) {
        Bounty storage b = bounties[jobId];
        return (b.buyer, b.seller, b.amount, b.deadline, b.resultHash, b.status);
    }

    // ============ 声誉层管理 ============

    /// @notice 设置声誉合约地址（仅 Owner）
    function setReputationContract(address _reputation) external onlyOwner {
        require(_reputation != address(0), "invalid address");
        reputation = IAgentReputation(_reputation);
    }
}
