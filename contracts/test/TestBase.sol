// SPDX-License-Identifier: MIT
pragma solidity ^0.8.26;

interface Vm {
    function prank(address) external;
    function expectRevert(bytes calldata) external;
}

contract TestBase {
    Vm internal constant vm = Vm(address(uint160(uint256(keccak256("hevm cheat code")))));

    function assertEq(uint256 a, uint256 b, string memory message) internal pure {
        require(a == b, message);
    }

    function assertEq(address a, address b, string memory message) internal pure {
        require(a == b, message);
    }

    function assertTrue(bool value, string memory message) internal pure {
        require(value, message);
    }
}
