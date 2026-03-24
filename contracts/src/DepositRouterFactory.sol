// SPDX-License-Identifier: MIT
pragma solidity ^0.8.26;

import "./DepositRouter.sol";

contract DepositRouterFactory {
    address public immutable vault;
    address public immutable token;
    address public owner;

    mapping(uint256 => address) public routerOfUser;

    event RouterCreated(uint256 indexed userId, address indexed router, bytes32 indexed salt);

    modifier onlyOwner() {
        require(msg.sender == owner, "NOT_OWNER");
        _;
    }

    constructor(address owner_, address vault_, address token_) {
        require(owner_ != address(0), "ZERO_OWNER");
        owner = owner_;
        vault = vault_;
        token = token_;
    }

    function createRouter(uint256 userId, bytes32 salt) external onlyOwner returns (address router) {
        require(routerOfUser[userId] == address(0), "ROUTER_EXISTS");
        router = address(new DepositRouter{salt: salt}(userId, vault, token, owner));
        routerOfUser[userId] = router;
        emit RouterCreated(userId, router, salt);
    }

    function predictRouter(uint256 userId, bytes32 salt) external view returns (address predicted) {
        bytes memory bytecode = abi.encodePacked(type(DepositRouter).creationCode, abi.encode(userId, vault, token, owner));
        bytes32 hash = keccak256(abi.encodePacked(bytes1(0xff), address(this), salt, keccak256(bytecode)));
        predicted = address(uint160(uint256(hash)));
    }
}
