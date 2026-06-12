// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/**
 * @title AEPReputation
 * @notice 链上 Agent 声誉层 — ERC-8004 风格去中心化身份评分
 * @dev 评分计算在链下（Go backend）完成，链上只存最终分数和历史记录
 *
 * 关键设计：
 *   - updateScore 由 authorizedCallers 调用，传入计算好的最终分数
 *   - paramHash 存储评估参数的哈希，供第三方验证分数真实性
 *   - processedTasks 防止同一任务被重复处理
 *   - 历史记录可追溯（含分数变更原因、来源、参数哈希）
 */
contract AEPReputation {
    struct ReputationEvent {
        uint256 taskId;
        int256 delta;
        uint256 timestamp;
        string reason;
        address source;
        bytes32 paramHash; // 评估参数哈希，供外部验证
    }

    /// @notice 当前声誉分数
    mapping(address => uint256) public scores;

    /// @notice Agent 的历史记录
    mapping(address => ReputationEvent[]) public history;

    /// @notice 任务去重 (agent => taskId => processed)
    mapping(address => mapping(uint256 => bool)) public processedTasks;

    /// @notice 授权调用者白名单
    mapping(address => bool) public authorizedCallers;

    /// @notice 合约拥有者（可管理授权列表）
    address public owner;

    // ============ 事件 ============

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
    event OwnershipTransferred(address indexed previousOwner, address indexed newOwner);

    // ============ 修饰器 ============

    modifier onlyOwner() {
        require(msg.sender == owner, "AEPReputation: not owner");
        _;
    }

    modifier onlyAuthorized() {
        require(
            authorizedCallers[msg.sender] || msg.sender == owner,
            "AEPReputation: not authorized"
        );
        _;
    }

    // ============ 构造函数 ============

    /// @param _bountyContract 初始授权合约地址（通常是 AEPBounty）
    constructor(address _bountyContract) {
        require(_bountyContract != address(0), "AEPReputation: invalid address");
        owner = msg.sender;
        authorizedCallers[_bountyContract] = true;
        emit AuthorizedCallerAdded(_bountyContract);
    }

    // ============ 权限管理 ============

    /// @notice 添加授权调用者
    function addAuthorizedCaller(address caller) external onlyOwner {
        require(caller != address(0), "AEPReputation: invalid address");
        authorizedCallers[caller] = true;
        emit AuthorizedCallerAdded(caller);
    }

    /// @notice 移除授权调用者
    function removeAuthorizedCaller(address caller) external onlyOwner {
        require(caller != address(0), "AEPReputation: invalid address");
        require(authorizedCallers[caller], "AEPReputation: caller not authorized");
        authorizedCallers[caller] = false;
        emit AuthorizedCallerRemoved(caller);
    }

    /// @notice 转移合约所有权
    function transferOwnership(address newOwner) external onlyOwner {
        require(newOwner != address(0), "AEPReputation: invalid address");
        emit OwnershipTransferred(owner, newOwner);
        owner = newOwner;
    }

    // ============ 核心函数 ============

    /// @notice 更新 Agent 声誉分数
    /// @param agent 目标 Agent 地址
    /// @param taskId 任务 ID（防重放）
    /// @param newScore 计算后的新分数（0-100）
    /// @param delta 分数变化量（正/负）
    /// @param reason 变更原因（如 "delivery_passed", "delivery_slashed"）
    /// @param paramHash 评估参数的 SHA256 哈希（可验证性）
    function updateScore(
        address agent,
        uint256 taskId,
        uint256 newScore,
        int256 delta,
        string calldata reason,
        bytes32 paramHash
    ) external onlyAuthorized {
        require(!processedTasks[agent][taskId], "AEPReputation: task already processed");
        processedTasks[agent][taskId] = true;

        require(newScore <= 100, "AEPReputation: score out of range");
        require(delta == int256(newScore) - int256(scores[agent]), "AEPReputation: delta mismatch");

        scores[agent] = newScore;

        history[agent].push(ReputationEvent({
            taskId: taskId,
            delta: delta,
            timestamp: block.timestamp,
            reason: reason,
            source: msg.sender,
            paramHash: paramHash
        }));

        emit ScoreUpdated(agent, newScore, delta, taskId, reason, msg.sender, paramHash);
    }

    // ============ 查询函数 ============

    /// @notice 查询 Agent 当前声誉分
    function getScore(address agent) external view returns (uint256) {
        return scores[agent];
    }

    /// @notice 批量查询声誉分
    function getScores(address[] calldata agents) external view returns (uint256[] memory) {
        uint256[] memory result = new uint256[](agents.length);
        for (uint256 i = 0; i < agents.length; i++) {
            result[i] = scores[agents[i]];
        }
        return result;
    }

    /// @notice 查询 Agent 的历史记录（分页）
    /// @param agent 目标 Agent 地址
    /// @param offset 偏移量
    /// @param limit 返回条数上限
    /// @return 历史事件数组
    function getHistory(
        address agent,
        uint256 offset,
        uint256 limit
    ) external view returns (ReputationEvent[] memory) {
        uint256 len = history[agent].length;
        if (offset >= len) return new ReputationEvent[](0);

        uint256 end = offset + limit;
        if (end > len) end = len;

        ReputationEvent[] memory result = new ReputationEvent[](end - offset);
        for (uint256 i = offset; i < end; i++) {
            result[i - offset] = history[agent][i];
        }
        return result;
    }

    /// @notice 查询 Agent 的任务是否已处理
    function isTaskProcessed(address agent, uint256 taskId) external view returns (bool) {
        return processedTasks[agent][taskId];
    }

    /// @notice 获取 Agent 的历史记录总数
    function getHistoryCount(address agent) external view returns (uint256) {
        return history[agent].length;
    }
}
